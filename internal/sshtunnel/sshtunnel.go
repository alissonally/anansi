// Package sshtunnel opens SSH connections used to reach database servers
// (HeidiSQL "SSH tunnel"). The database driver dials through the SSH client
// directly, so no local port is opened.
//
// Host keys are always verified: against the user's ~/.ssh/known_hosts and the
// application's own known_hosts file. An unknown host fails with
// *UnknownHostError, carrying the fingerprint for the user to confirm; a
// changed key fails hard.
package sshtunnel

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Auth methods.
const (
	AuthPassword = "password"
	AuthKey      = "key"
	AuthAgent    = "agent"
)

// Config describes an SSH endpoint.
type Config struct {
	Host string
	Port int
	User string
	// Auth: password | key | agent.
	Auth string
	// Secret is the password (password auth) or the key passphrase.
	Secret  string
	KeyFile string
	Timeout time.Duration
	// KeepAlive sends keepalive@openssh.com requests (0 = 30 s).
	KeepAlive time.Duration
}

// Address returns host:port.
func (c Config) Address() string {
	port := c.Port
	if port == 0 {
		port = 22
	}
	return net.JoinHostPort(strings.TrimSpace(c.Host), strconv.Itoa(port))
}

// Validate checks the fields.
func (c Config) Validate() error {
	var problems []string
	if strings.TrimSpace(c.Host) == "" || strings.ContainsAny(c.Host, " /\\?#@") {
		problems = append(problems, "host SSH inválido")
	}
	if c.Port < 0 || c.Port > 65535 {
		problems = append(problems, "porta SSH inválida")
	}
	if strings.TrimSpace(c.User) == "" {
		problems = append(problems, "o usuário SSH é obrigatório")
	}
	switch c.Auth {
	case AuthPassword, AuthAgent:
	case AuthKey:
		if c.KeyFile == "" {
			problems = append(problems, "informe o arquivo da chave privada SSH")
		} else if st, err := os.Stat(c.KeyFile); err != nil || !st.Mode().IsRegular() {
			problems = append(problems, "arquivo da chave SSH não encontrado: "+c.KeyFile)
		}
	default:
		problems = append(problems, "método de autenticação SSH inválido")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// UnknownHostError reports a host key that is not in any known_hosts file.
type UnknownHostError struct {
	Host        string `json:"host"` // known_hosts form: host or [host]:port
	KeyType     string `json:"keyType"`
	Fingerprint string `json:"fingerprint"` // SHA256:…
	key         ssh.PublicKey
}

func (e *UnknownHostError) Error() string {
	return fmt.Sprintf("a chave do servidor SSH %s não é conhecida (%s %s)", e.Host, e.KeyType, e.Fingerprint)
}

// ErrNeedSecret means the password or key passphrase is required.
var ErrNeedSecret = errors.New("senha SSH necessária")

// Fingerprint formats a key like ssh-keygen -l (SHA256).
func Fingerprint(k ssh.PublicKey) string {
	sum := sha256.Sum256(k.Marshal())
	return "SHA256:" + strings.TrimRight(base64.StdEncoding.EncodeToString(sum[:]), "=")
}

// HostKeys checks host keys against known_hosts files.
type HostKeys struct {
	// AppFile is the application's known_hosts (written on trust).
	AppFile string
	// UserFile is ~/.ssh/known_hosts (read only); "" disables it.
	UserFile string

	mu      sync.Mutex
	pending map[string]ssh.PublicKey // host -> last unknown key seen
}

// NewHostKeys uses appFile and the user's ~/.ssh/known_hosts.
func NewHostKeys(appFile string) *HostKeys {
	h := &HostKeys{AppFile: appFile, pending: map[string]ssh.PublicKey{}}
	if home, err := os.UserHomeDir(); err == nil {
		h.UserFile = filepath.Join(home, ".ssh", "known_hosts")
	}
	return h
}

func (h *HostKeys) files() []string {
	var out []string
	for _, f := range []string{h.UserFile, h.AppFile} {
		if f == "" {
			continue
		}
		if _, err := os.Stat(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

func (h *HostKeys) callback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	host := knownhosts.Normalize(hostname)
	if files := h.files(); len(files) > 0 {
		cb, err := knownhosts.New(files...)
		if err != nil {
			return fmt.Errorf("known_hosts inválido: %w", err)
		}
		err = cb(hostname, remote, key)
		var ke *knownhosts.KeyError
		switch {
		case err == nil:
			return nil
		case errors.As(err, &ke) && len(ke.Want) > 0:
			return fmt.Errorf("A CHAVE DO SERVIDOR SSH %s MUDOU (agora %s %s). Pode ser um ataque man-in-the-middle; confira com o administrador e corrija o known_hosts", host, key.Type(), Fingerprint(key))
		case errors.As(err, &ke):
			// unknown: fall through
		default:
			return err
		}
	}
	h.mu.Lock()
	h.pending[host] = key
	h.mu.Unlock()
	return &UnknownHostError{Host: host, KeyType: key.Type(), Fingerprint: Fingerprint(key), key: key}
}

// Trust records the key last offered by host (as reported by an
// UnknownHostError) in the application known_hosts. Only a key the server
// actually presented can be trusted, and only when its fingerprint matches.
func (h *HostKeys) Trust(host, fingerprint string) error {
	h.mu.Lock()
	key, ok := h.pending[host]
	h.mu.Unlock()
	if !ok || Fingerprint(key) != fingerprint {
		return errors.New("nenhuma chave pendente para este servidor; conecte novamente")
	}
	if err := os.MkdirAll(filepath.Dir(h.AppFile), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(h.AppFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(knownhosts.Line([]string{host}, key) + "\n"); err != nil {
		return err
	}
	h.mu.Lock()
	delete(h.pending, host)
	h.mu.Unlock()
	return nil
}

// Known lists the hosts of the application known_hosts file.
func (h *HostKeys) Known() []string {
	f, err := os.Open(h.AppFile)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) >= 3 && !strings.HasPrefix(fields[0], "#") {
			out = append(out, fields[0]+" "+fields[1])
		}
	}
	return out
}

// Tunnel is an open SSH client.
type Tunnel struct {
	client *ssh.Client
	stop   chan struct{}
	once   sync.Once
}

func authMethods(c Config) ([]ssh.AuthMethod, func(), error) {
	switch c.Auth {
	case AuthPassword:
		if c.Secret == "" {
			return nil, nil, ErrNeedSecret
		}
		pw := c.Secret
		return []ssh.AuthMethod{ssh.Password(pw), ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
			ans := make([]string, len(qs))
			for i := range ans {
				ans[i] = pw
			}
			return ans, nil
		})}, nil, nil
	case AuthKey:
		pem, err := os.ReadFile(c.KeyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("não foi possível ler a chave SSH: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(pem)
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			if c.Secret == "" {
				return nil, nil, ErrNeedSecret
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(pem, []byte(c.Secret))
		}
		if err != nil {
			return nil, nil, fmt.Errorf("chave SSH inválida: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil, nil
	case AuthAgent:
		sock := os.Getenv("SSH_AUTH_SOCK")
		if sock == "" {
			return nil, nil, errors.New("agente SSH não encontrado (SSH_AUTH_SOCK vazio)")
		}
		conn, err := net.Dial("unix", sock)
		if err != nil {
			return nil, nil, fmt.Errorf("não foi possível falar com o agente SSH: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeysCallback(agent.NewClient(conn).Signers)}, func() { _ = conn.Close() }, nil
	}
	return nil, nil, errors.New("método de autenticação SSH inválido")
}

// Open connects and authenticates.
func Open(ctx context.Context, c Config, keys *HostKeys) (*Tunnel, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	auth, cleanup, err := authMethods(c)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	cfg := &ssh.ClientConfig{User: c.User, Auth: auth, HostKeyCallback: keys.callback, Timeout: timeout}
	d := net.Dialer{Timeout: timeout}
	raw, err := d.DialContext(ctx, "tcp", c.Address())
	if err != nil {
		return nil, fmt.Errorf("não foi possível conectar ao servidor SSH %s: %w", c.Address(), err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(dl)
	} else {
		_ = raw.SetDeadline(time.Now().Add(timeout))
	}
	sc, chans, reqs, err := ssh.NewClientConn(raw, c.Address(), cfg)
	if err != nil {
		_ = raw.Close()
		var uh *UnknownHostError
		if errors.As(err, &uh) {
			return nil, uh
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, errors.New("o servidor SSH recusou a autenticação")
		}
		return nil, fmt.Errorf("falha no SSH: %w", err)
	}
	_ = raw.SetDeadline(time.Time{})
	t := &Tunnel{client: ssh.NewClient(sc, chans, reqs), stop: make(chan struct{})}
	ka := c.KeepAlive
	if ka <= 0 {
		ka = 30 * time.Second
	}
	go t.keepAlive(ka)
	return t, nil
}

func (t *Tunnel) keepAlive(every time.Duration) {
	tk := time.NewTicker(every)
	defer tk.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-tk.C:
			if _, _, err := t.client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				return
			}
		}
	}
}

// DialContext opens a TCP connection from the SSH server to addr.
func (t *Tunnel) DialContext(ctx context.Context, _, addr string) (net.Conn, error) {
	c, err := t.client.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return &deadlineConn{Conn: c}, nil
}

// Close closes the SSH client (and every connection through it).
func (t *Tunnel) Close() error {
	var err error
	t.once.Do(func() {
		close(t.stop)
		err = t.client.Close()
	})
	return err
}
