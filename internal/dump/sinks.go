package dump

import (
	"bufio"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// countingWriter counts bytes written (progress of file sinks).
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// textSink writes SQL text; compound statements are wrapped in DELIMITER.
type textSink struct {
	w      *bufio.Writer
	closer []io.Closer
	cw     *countingWriter
}

func (s *textSink) Comment(text string) error {
	for _, l := range strings.Split(text, "\n") {
		if _, err := s.w.WriteString("-- " + l + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func (s *textSink) Exec(st Stmt) error {
	var err error
	if st.Compound {
		_, err = s.w.WriteString("DELIMITER ;;\n" + st.SQL + ";;\nDELIMITER ;\n")
	} else {
		_, err = s.w.WriteString(st.SQL + ";\n")
	}
	return err
}

func (s *textSink) Database(name string) error {
	_, err := s.w.WriteString("\n")
	return err
}

func (s *textSink) Close() error {
	if err := s.w.Flush(); err != nil {
		return err
	}
	for i := len(s.closer) - 1; i >= 0; i-- {
		if err := s.closer[i].Close(); err != nil {
			return err
		}
	}
	s.closer = nil
	return nil
}

// Written reports the bytes written so far (before compression).
func (s *textSink) Written() int64 { return s.cw.n }

// NewFileSink writes one .sql file (gzip-compressed when the name ends in
// .gz). The file is created with owner-only permissions.
func NewFileSink(path string) (Sink, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	s := &textSink{closer: []io.Closer{f}}
	var w io.Writer = f
	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		gz := gzip.NewWriter(f)
		s.closer = append(s.closer, gz)
		w = gz
	}
	s.cw = &countingWriter{w: w}
	s.w = bufio.NewWriterSize(s.cw, 256<<10)
	return s, nil
}

// NewWriterSink writes SQL text to w (tests, clipboard).
func NewWriterSink(w io.Writer) Sink {
	cw := &countingWriter{w: w}
	return &textSink{w: bufio.NewWriter(cw), cw: cw}
}

// dirSink writes <dir>/<db>/<kind>-<name>.sql, each file with the session
// header and footer so it can be run on its own.
type dirSink struct {
	root   string
	db     string
	files  map[string]*textSink
	common []string // statements written to every file (header)
}

// NewDirSink writes one file per object below dir.
func NewDirSink(dir string) (Sink, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &dirSink{root: dir, files: map[string]*textSink{}}, nil
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '_'
		}
		return r
	}, s)
}

func (s *dirSink) file(obj string) (*textSink, error) {
	key := s.db + "\x00" + obj
	if f, ok := s.files[key]; ok {
		return f, nil
	}
	dir := filepath.Join(s.root, safeName(s.db))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	name := obj
	if name == "" {
		name = "database"
	}
	fs, err := NewFileSink(filepath.Join(dir, safeName(name)+".sql"))
	if err != nil {
		return nil, err
	}
	f := fs.(*textSink)
	for _, h := range s.common {
		if err := f.Exec(Stmt{SQL: h}); err != nil {
			return nil, err
		}
	}
	if s.db != "" {
		if err := f.Exec(Stmt{SQL: "USE `" + strings.ReplaceAll(s.db, "`", "``") + "`"}); err != nil {
			return nil, err
		}
	}
	s.files[key] = f
	return f, nil
}

func (s *dirSink) Comment(string) error { return nil }

func (s *dirSink) Exec(st Stmt) error {
	if st.Object == "" && strings.HasPrefix(st.SQL, "/*!") {
		// Session header: replayed at the top of every file (the global
		// footer is written by Close into each file).
		if s.db == "" {
			s.common = append(s.common, st.SQL)
		}
		return nil
	}
	if st.Object == "" && strings.HasPrefix(st.SQL, "USE ") {
		return nil
	}
	f, err := s.file(st.Object)
	if err != nil {
		return err
	}
	return f.Exec(st)
}

func (s *dirSink) Database(name string) error {
	s.db = name
	return nil
}

func (s *dirSink) Close() error {
	var first error
	for _, f := range s.files {
		for _, l := range footer() {
			_ = f.Exec(Stmt{SQL: l})
		}
		if err := f.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// ExecSink runs the statements on a target connection (copy to another
// database or server). FailFast stops at the first error; otherwise errors
// are reported to OnError and the copy continues.
type ExecSink struct {
	Ctx     context.Context
	Conn    *sql.Conn
	OnError func(stmt string, err error) bool // return false to stop
	// Executed counts successful statements.
	Executed int
}

// Comment implements Sink.
func (e *ExecSink) Comment(string) error { return nil }

// Exec implements Sink.
func (e *ExecSink) Exec(st Stmt) error {
	_, err := e.Conn.ExecContext(e.Ctx, st.SQL)
	if err != nil {
		if e.OnError != nil && e.OnError(st.SQL, err) {
			return nil
		}
		s := st.SQL
		if len(s) > 300 {
			s = s[:300] + "…"
		}
		return fmt.Errorf("%w (instrução: %s)", err, s)
	}
	e.Executed++
	return nil
}

// Database implements Sink.
func (e *ExecSink) Database(string) error { return nil }

// Close implements Sink.
func (e *ExecSink) Close() error { return nil }
