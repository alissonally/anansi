package connections

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"anansi/internal/apperr"
	"anansi/internal/database"
	"anansi/internal/database/sqllog"
	"anansi/internal/security"
	"anansi/internal/sshtunnel"
)

// Session is a live connection pool opened from a profile. Several SQL tabs
// share one session; each tab pins at most one physical connection from the
// pool (see the query package).
type Session struct {
	ID       string // equal to the profile id: one session per profile
	Profile  Profile
	DB       *sql.DB
	Driver   database.Driver
	Server   database.ServerInfo
	OpenedAt time.Time

	password string // kept only to redact error messages
	tunnel   *sshtunnel.Tunnel
	stop     chan struct{}
}

// close releases the pool, the keep-alive and the SSH tunnel.
func (s *Session) close() error {
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
	var err error
	if s.DB != nil {
		if r, ok := s.Driver.(database.Releaser); ok {
			r.Release(s.DB)
		}
		err = s.DB.Close()
	}
	if s.tunnel != nil {
		_ = s.tunnel.Close()
	}
	return err
}

// keepAlive pings the server (HeidiSQL "ping every N seconds") so idle
// sessions, NAT entries and tunnels are not dropped.
func (s *Session) keepAlive(every time.Duration) {
	s.stop = make(chan struct{})
	stop, db := s.stop, s.DB
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				ctx, cancel := context.WithTimeout(context.Background(), every)
				_ = db.PingContext(ctx)
				cancel()
			}
		}
	}()
}

type sshSecretKey struct{}

// WithSSHSecret attaches the SSH password or key passphrase to ctx for the
// opener.
func WithSSHSecret(ctx context.Context, secret string) context.Context {
	return context.WithValue(ctx, sshSecretKey{}, secret)
}

// OpenOption configures RegistryOpener.
type OpenOption func(*openOptions)

type openOptions struct{ hostKeys *sshtunnel.HostKeys }

// WithHostKeys sets the known_hosts used to verify SSH servers.
func WithHostKeys(k *sshtunnel.HostKeys) OpenOption {
	return func(o *openOptions) { o.hostKeys = k }
}

// Redact removes the session password from s.
func (s *Session) Redact(msg string) string { return security.Redact(msg, s.password) }

// SessionState is the frontend-facing view of a session.
type SessionState struct {
	ProfileID string              `json:"profileId"`
	Status    string              `json:"status"` // connecting | connected
	Server    database.ServerInfo `json:"server"`
	OpenedAt  time.Time           `json:"openedAt"`
}

type pending struct {
	done chan struct{}
	sess *Session
	err  error
}

// Opener abstracts opening a pool so the manager can be tested without a
// server.
type Opener func(ctx context.Context, p Profile, password string) (*Session, error)

// Manager tracks open sessions. All methods are safe for concurrent use and
// never hold the lock while performing network I/O.
type Manager struct {
	mu         sync.Mutex
	sessions   map[string]*Session
	connecting map[string]*pending
	closed     bool
	open       Opener
	onClose    []func(id string)
}

// NewManager creates a manager that opens sessions with open.
func NewManager(open Opener) *Manager {
	return &Manager{sessions: map[string]*Session{}, connecting: map[string]*pending{}, open: open}
}

// RegistryOpener builds an Opener that uses drivers from reg. When logFor is
// not nil, every statement sent through a session's pool is reported to the
// function it returns for that profile (SQL log panel).
func RegistryOpener(reg *database.Registry, logFor func(profileID string) sqllog.Func, opts ...OpenOption) Opener {
	var o openOptions
	for _, f := range opts {
		f(&o)
	}
	return func(ctx context.Context, p Profile, password string) (*Session, error) {
		drv, err := reg.Get(p.Kind)
		if err != nil {
			return nil, apperr.New(apperr.CodeValidation, err.Error())
		}
		params := p.ConnectParams(password)
		if logFor != nil {
			params.Log = logFor(p.ID)
		}
		var tun *sshtunnel.Tunnel
		if p.SSH.Enabled {
			secret, _ := ctx.Value(sshSecretKey{}).(string)
			tun, err = openTunnel(ctx, p, secret, o.hostKeys)
			if err != nil {
				return nil, err
			}
			params.Dial = tun.DialContext
		}
		fail := func(err error) (*Session, error) {
			if tun != nil {
				_ = tun.Close()
			}
			return nil, err
		}
		db, err := drv.Open(ctx, params)
		if err != nil {
			return fail(redactErr(drv.DescribeError(err), password))
		}
		info, err := drv.ServerInfo(ctx, db)
		if err != nil {
			_ = db.Close()
			return fail(redactErr(drv.DescribeError(err), password))
		}
		if p.TLS.Mode != database.TLSDisabled && p.TLS.Mode != database.TLSPreferred && !info.TLSActive {
			// Defensive: the driver should already have refused this.
			_ = db.Close()
			return fail(apperr.New(apperr.CodeDatabase, "a conexão exige TLS, mas o servidor não negociou TLS"))
		}
		s := &Session{ID: p.ID, Profile: p, DB: db, Driver: drv, Server: info, OpenedAt: time.Now(), password: password, tunnel: tun}
		if p.KeepAliveSeconds > 0 {
			s.keepAlive(time.Duration(p.KeepAliveSeconds) * time.Second)
		}
		return s, nil
	}
}

func openTunnel(ctx context.Context, p Profile, secret string, keys *sshtunnel.HostKeys) (*sshtunnel.Tunnel, error) {
	if keys == nil {
		return nil, apperr.New(apperr.CodeInternal, "verificação de chaves SSH não configurada")
	}
	cfg := p.TunnelConfig(secret)
	if p.KeepAliveSeconds > 0 {
		cfg.KeepAlive = time.Duration(p.KeepAliveSeconds) * time.Second
	}
	tun, err := sshtunnel.Open(ctx, cfg, keys)
	if err == nil {
		return tun, nil
	}
	var uh *sshtunnel.UnknownHostError
	switch {
	case errors.As(err, &uh):
		return nil, &apperr.Error{Code: apperr.CodeSSHHostUnknown, Message: uh.Error(), Details: uh}
	case errors.Is(err, sshtunnel.ErrNeedSecret):
		what := "a senha SSH"
		if p.SSH.Auth == sshtunnel.AuthKey {
			what = "a frase secreta da chave SSH"
		}
		return nil, &apperr.Error{Code: apperr.CodeSSHSecretRequired, Message: "Informe " + what + "."}
	}
	return nil, apperr.New(apperr.CodeDatabase, "túnel SSH: "+security.Redact(err.Error(), secret))
}

func redactErr(de *database.DBError, password string) error {
	c := *de
	c.Message = security.Redact(c.Message, password)
	c.Near = security.Redact(c.Near, password)
	return apperr.FromDB(&c)
}

// OnClose registers a hook called (without locks held) when a session is
// closed, before its pool is closed. Hooks must not call back into Close.
func (m *Manager) OnClose(fn func(id string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onClose = append(m.onClose, fn)
}

// Connect opens a session for p or returns the existing one. Concurrent calls
// for the same profile share a single connection attempt.
func (m *Manager) Connect(ctx context.Context, p Profile, password string) (*Session, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, apperr.New(apperr.CodeInternal, "o aplicativo está sendo encerrado")
	}
	if s, ok := m.sessions[p.ID]; ok {
		m.mu.Unlock()
		return s, nil
	}
	if pd, ok := m.connecting[p.ID]; ok {
		m.mu.Unlock()
		select {
		case <-pd.done:
			return pd.sess, pd.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	pd := &pending{done: make(chan struct{})}
	m.connecting[p.ID] = pd
	m.mu.Unlock()

	sess, err := m.open(ctx, p, password)

	m.mu.Lock()
	delete(m.connecting, p.ID)
	if err == nil && m.closed {
		m.mu.Unlock()
		_ = sess.close()
		err = apperr.New(apperr.CodeInternal, "o aplicativo está sendo encerrado")
		sess = nil
		m.mu.Lock()
	}
	if err == nil {
		m.sessions[p.ID] = sess
	}
	pd.sess, pd.err = sess, err
	close(pd.done)
	m.mu.Unlock()
	return sess, err
}

func (m *Manager) isOpen(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sessions[id]
	return ok
}

// Get returns an open session.
func (m *Manager) Get(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, apperr.New(apperr.CodeNotConnected, "a conexão não está aberta")
	}
	return s, nil
}

// IsConnecting reports whether a connection attempt is in progress.
func (m *Manager) IsConnecting(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.connecting[id]
	return ok
}

// States lists the open and connecting sessions.
func (m *Manager) States() []SessionState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SessionState, 0, len(m.sessions)+len(m.connecting))
	for id, s := range m.sessions {
		out = append(out, SessionState{ProfileID: id, Status: "connected", Server: s.Server, OpenedAt: s.OpenedAt})
	}
	for id := range m.connecting {
		out = append(out, SessionState{ProfileID: id, Status: "connecting"})
	}
	return out
}

// Disconnect closes a session. Closing a session that is not open is a no-op.
func (m *Manager) Disconnect(id string) error {
	s := m.detach(id)
	if s == nil {
		return nil
	}
	return s.close()
}

// detach removes a session and runs the close hooks.
func (m *Manager) detach(id string) *Session {
	m.mu.Lock()
	s, ok := m.sessions[id]
	delete(m.sessions, id)
	hooks := append([]func(string){}, m.onClose...)
	m.mu.Unlock()
	if !ok {
		return nil
	}
	for _, h := range hooks {
		h(id)
	}
	return s
}

// CloseAll closes every session; used on application shutdown. It waits for
// in-flight connection attempts up to ctx's deadline.
func (m *Manager) CloseAll(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	var waits []chan struct{}
	for _, pd := range m.connecting {
		waits = append(waits, pd.done)
	}
	m.mu.Unlock()
	for _, w := range waits {
		select {
		case <-w:
		case <-ctx.Done():
		}
	}
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	var errs []error
	for _, id := range ids {
		if err := m.Disconnect(id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
