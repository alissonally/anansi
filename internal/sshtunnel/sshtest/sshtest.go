// Package sshtest runs a minimal in-process SSH server that accepts
// password or public key logins and "direct-tcpip" forwarding, for tests.
package sshtest

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/crypto/ssh"
)

// Server is a test SSH server.
type Server struct {
	Addr     string
	HostKey  ssh.PublicKey
	User     string
	Password string
	// AuthorizedKey, when set, is accepted for public key logins.
	AuthorizedKey ssh.PublicKey
	// Forwards counts direct-tcpip channels opened.
	Forwards atomic.Int64

	ln   net.Listener
	wg   sync.WaitGroup
	mu   sync.Mutex
	open []net.Conn
}

// Start listens on 127.0.0.1 with a fresh host key.
func Start(user, password string) (*Server, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{Addr: ln.Addr().String(), HostKey: signer.PublicKey(), User: user, Password: password, ln: ln}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if c.User() == s.User && s.Password != "" && string(pw) == s.Password {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == s.User && s.AuthorizedKey != nil && bytes.Equal(k.Marshal(), s.AuthorizedKey.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
	}
	cfg.AddHostKey(signer)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.track(c)
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.serve(c, cfg)
			}()
		}
	}()
	return s, nil
}

func (s *Server) track(c net.Conn) {
	s.mu.Lock()
	s.open = append(s.open, c)
	s.mu.Unlock()
}

// Port returns the listening port.
func (s *Server) Port() int {
	_, p, _ := net.SplitHostPort(s.Addr)
	n, _ := strconv.Atoi(p)
	return n
}

func (s *Server) serve(c net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		_ = c.Close()
		return
	}
	defer sc.Close()
	go func() {
		for r := range reqs {
			if r.WantReply {
				_ = r.Reply(r.Type == "keepalive@openssh.com", nil)
			}
		}
	}()
	for nc := range chans {
		if nc.ChannelType() != "direct-tcpip" {
			_ = nc.Reject(ssh.UnknownChannelType, "only direct-tcpip")
			continue
		}
		// host string, port uint32, origin string, origin port uint32
		var msg struct {
			Host       string
			Port       uint32
			OriginHost string
			OriginPort uint32
		}
		if err := ssh.Unmarshal(nc.ExtraData(), &msg); err != nil {
			_ = nc.Reject(ssh.ConnectionFailed, "bad request")
			continue
		}
		target, err := net.Dial("tcp", net.JoinHostPort(msg.Host, strconv.Itoa(int(msg.Port))))
		if err != nil {
			_ = nc.Reject(ssh.ConnectionFailed, err.Error())
			continue
		}
		ch, creqs, err := nc.Accept()
		if err != nil {
			_ = target.Close()
			continue
		}
		s.Forwards.Add(1)
		go ssh.DiscardRequests(creqs)
		go func() {
			defer ch.Close()
			defer target.Close()
			done := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(target, ch); done <- struct{}{} }()
			go func() { _, _ = io.Copy(ch, target); done <- struct{}{} }()
			<-done
		}()
	}
}

// Close stops the server and drops its connections.
func (s *Server) Close() {
	_ = s.ln.Close()
	s.mu.Lock()
	for _, c := range s.open {
		_ = c.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
}
