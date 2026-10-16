// Package sqlfile executes SQL scripts of any size by streaming them:
// the file is read in chunks, split incrementally (DELIMITER aware) and
// each statement is sent as soon as it is complete (HeidiSQL "Run SQL
// file", which loads 20 MB chunks).
package sqlfile

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"dbstudio/internal/sqllex"
)

const chunkSize = 4 << 20

// Progress receives advancement (bytes read).
type Progress interface {
	Set(done, total int64)
	Message(s string)
	Warn(msg string)
}

// Result summarises a run.
type Result struct {
	Statements int64  `json:"statements"`
	Errors     int64  `json:"errors"`
	FirstError string `json:"firstError,omitempty"`
}

// Exec runs one statement.
type Exec func(ctx context.Context, st sqllex.Statement) error

// Run executes every statement read from r. With stopOnError the first
// failure ends the run (and is returned); otherwise failures are counted
// and reported to the progress.
func Run(ctx context.Context, r io.Reader, total int64, exec Exec, p Progress, stopOnError bool) (Result, error) {
	var res Result
	br := bufio.NewReaderSize(r, 64<<10)
	// Skip a UTF-8 byte order mark.
	if b, err := br.Peek(3); err == nil && string(b) == "\xef\xbb\xbf" {
		_, _ = br.Discard(3)
	}
	tmp := make([]byte, chunkSize)
	delim := ";"
	var read int64
	pending := "" // unfinished tail of the previous chunk
	runStmt := func(st sqllex.Statement) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		res.Statements++
		if err := exec(ctx, st); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			res.Errors++
			msg := fmt.Sprintf("instrução %d: %v", res.Statements, err)
			if res.FirstError == "" {
				res.FirstError = msg
			}
			if stopOnError {
				return errors.New(msg)
			}
			p.Warn(msg)
		}
		return nil
	}
	for {
		n, rerr := io.ReadFull(br, tmp)
		read += int64(n)
		eof := rerr == io.EOF || rerr == io.ErrUnexpectedEOF
		if rerr != nil && !eof {
			return res, rerr
		}
		// A multi-byte character cut at the end of the chunk stays in the
		// unfinished remainder and is completed by the next chunk.
		chunk := tmp[:n]
		text := pending + string(chunk)
		if eof {
			if strings.TrimSpace(text) != "" {
				prefix := ""
				if delim != ";" {
					prefix = "DELIMITER " + delim + "\n"
				}
				for _, st := range sqllex.Split(prefix + text) {
					if err := runStmt(st); err != nil {
						return res, err
					}
				}
			}
			p.Set(read, total)
			return res, nil
		}
		stmts, rest, d := sqllex.SplitPartial(text, delim)
		for _, st := range stmts {
			if err := runStmt(st); err != nil {
				return res, err
			}
		}
		pending, delim = text[rest:], d
		p.Set(read, total)
	}
}
