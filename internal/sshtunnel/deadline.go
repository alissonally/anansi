package sshtunnel

import (
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// deadlineConn adds deadlines to SSH channels (which return an error from
// SetDeadline). An expired deadline closes the channel: database drivers
// treat timeouts as fatal for the connection anyway.
type deadlineConn struct {
	net.Conn
	mu      sync.Mutex
	timers  [2]*time.Timer // read, write
	expired atomic.Bool
}

func (c *deadlineConn) set(i int, t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timers[i] != nil {
		c.timers[i].Stop()
		c.timers[i] = nil
	}
	if t.IsZero() {
		return nil
	}
	d := time.Until(t)
	if d <= 0 {
		d = 0
	}
	c.timers[i] = time.AfterFunc(d, func() {
		c.expired.Store(true)
		_ = c.Conn.Close()
	})
	return nil
}

func (c *deadlineConn) SetReadDeadline(t time.Time) error  { return c.set(0, t) }
func (c *deadlineConn) SetWriteDeadline(t time.Time) error { return c.set(1, t) }
func (c *deadlineConn) SetDeadline(t time.Time) error {
	_ = c.set(0, t)
	return c.set(1, t)
}

func (c *deadlineConn) wrapErr(err error) error {
	if err != nil && c.expired.Load() {
		return os.ErrDeadlineExceeded
	}
	return err
}

func (c *deadlineConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	return n, c.wrapErr(err)
}

func (c *deadlineConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	return n, c.wrapErr(err)
}

func (c *deadlineConn) Close() error {
	c.mu.Lock()
	for _, t := range c.timers {
		if t != nil {
			t.Stop()
		}
	}
	c.mu.Unlock()
	return c.Conn.Close()
}
