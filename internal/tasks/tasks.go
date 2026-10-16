// Package tasks runs long operations (exports, dumps, imports, SQL files,
// table maintenance, text search) in the background with throttled
// progress notifications and cancellation.
package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

// Status of a task.
const (
	Running   = "running"
	Done      = "done"
	Failed    = "error"
	Cancelled = "cancelled"
)

// Task is the frontend view of a background operation.
type Task struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	Done       int64     `json:"done"`
	Total      int64     `json:"total"` // 0 = unknown
	Unit       string    `json:"unit"`  // bytes | rows | statements | objects | tables
	Message    string    `json:"message,omitempty"`
	Error      string    `json:"error,omitempty"`
	Warnings   int       `json:"warnings"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	// Result is a short outcome shown when done (e.g. the file written).
	Result string `json:"result,omitempty"`
	// Path of the file produced, when any (offered to open the folder).
	Path string `json:"path,omitempty"`
	// Data is a structured result (maintenance messages, search hits…).
	Data any `json:"data,omitempty"`
}

// Progress is handed to a task function to report its state.
type Progress struct {
	m    *Manager
	id   string
	mu   sync.Mutex
	last time.Time
}

func (p *Progress) update(fn func(t *Task), force bool) {
	p.m.mu.Lock()
	t, ok := p.m.tasks[p.id]
	if ok {
		fn(t)
	}
	var snap Task
	if ok {
		snap = *t
	}
	p.m.mu.Unlock()
	if !ok {
		return
	}
	p.mu.Lock()
	due := force || time.Since(p.last) >= 200*time.Millisecond
	if due {
		p.last = time.Now()
	}
	p.mu.Unlock()
	if due && p.m.emit != nil {
		p.m.emit(snap)
	}
}

// Set reports done/total progress.
func (p *Progress) Set(done, total int64) {
	p.update(func(t *Task) { t.Done, t.Total = done, total }, false)
}

// Add increments done.
func (p *Progress) Add(n int64) { p.update(func(t *Task) { t.Done += n }, false) }

// Message sets the current step description.
func (p *Progress) Message(s string) { p.update(func(t *Task) { t.Message = s }, false) }

// Warn counts a non-fatal problem (e.g. a failed statement when continuing).
func (p *Progress) Warn(msg string) {
	p.update(func(t *Task) {
		t.Warnings++
		if msg != "" {
			t.Message = msg
		}
	}, false)
}

// SetData records a structured result (sent with the next update).
func (p *Progress) SetData(v any) { p.update(func(t *Task) { t.Data = v }, true) }

// SetPath records the produced file.
func (p *Progress) SetPath(path string) { p.update(func(t *Task) { t.Path = path }, false) }

// Func is the body of a task. It returns a short result text.
type Func func(ctx context.Context, p *Progress) (string, error)

// Manager tracks tasks.
type Manager struct {
	mu      sync.Mutex
	tasks   map[string]*Task
	cancels map[string]context.CancelFunc
	emit    func(Task)
	wg      sync.WaitGroup
}

// NewManager creates a manager; emit receives task snapshots.
func NewManager(emit func(Task)) *Manager {
	return &Manager{tasks: map[string]*Task{}, cancels: map[string]context.CancelFunc{}, emit: emit}
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Start runs fn in the background and returns the task id.
func (m *Manager) Start(kind, title, unit string, fn Func) string {
	ctx, cancel := context.WithCancel(context.Background())
	t := &Task{ID: newID(), Kind: kind, Title: title, Unit: unit, Status: Running, StartedAt: time.Now()}
	m.mu.Lock()
	m.tasks[t.ID] = t
	m.cancels[t.ID] = cancel
	m.mu.Unlock()
	p := &Progress{m: m, id: t.ID}
	p.update(func(*Task) {}, true)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		res, err := safeRun(ctx, fn, p)
		p.update(func(t *Task) {
			t.FinishedAt = time.Now()
			t.Result = res
			switch {
			case errors.Is(err, context.Canceled) || (err != nil && ctx.Err() != nil):
				t.Status = Cancelled
			case err != nil:
				t.Status = Failed
				t.Error = err.Error()
			default:
				t.Status = Done
				if t.Total > 0 && t.Done < t.Total && t.Unit == "bytes" {
					t.Done = t.Total
				}
			}
		}, true)
		m.mu.Lock()
		delete(m.cancels, t.ID)
		m.mu.Unlock()
	}()
	return t.ID
}

func safeRun(ctx context.Context, fn Func, p *Progress) (res string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("falha interna na tarefa")
		}
	}()
	return fn(ctx, p)
}

// Cancel stops a running task.
func (m *Manager) Cancel(id string) {
	m.mu.Lock()
	c := m.cancels[id]
	m.mu.Unlock()
	if c != nil {
		c()
	}
}

// List returns the tasks, newest first.
func (m *Manager) List() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

// Dismiss forgets a finished task.
func (m *Manager) Dismiss(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tasks[id]; ok && t.Status != Running {
		delete(m.tasks, id)
	}
}

// Close cancels every task and waits for them (bounded by ctx).
func (m *Manager) Close(ctx context.Context) {
	m.mu.Lock()
	for _, c := range m.cancels {
		c()
	}
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
