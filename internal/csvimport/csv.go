// Package csvimport reads delimited text files (HeidiSQL "Import CSV
// file"): a configurable streaming parser, a layout/type detector that
// proposes a table definition, and an importer that sends batched
// multi-row INSERTs with bound values.
package csvimport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// Format describes the file layout.
type Format struct {
	Separator string `json:"separator"` // one character, "\t" for tab
	Encloser  string `json:"encloser"`  // usually `"`, "" = none
	// Escape is the escape character inside enclosed fields: the encloser
	// itself (doubled quotes, RFC 4180) or "\\" (MySQL style).
	Escape string `json:"escape"`
	// Encoding: "utf-8" (default) or "latin1" (Windows-1252).
	Encoding string `json:"encoding"`
	// SkipLines are skipped before reading (e.g. 1 for a header line).
	SkipLines int `json:"skipLines"`
	// NullMarker: values equal to it are NULL (default \N).
	NullMarker string `json:"nullMarker"`
	EmptyNull  bool   `json:"emptyNull"`
}

func (f Format) sep() rune {
	if f.Separator == `\t` || f.Separator == "tab" {
		return '\t'
	}
	r, _ := utf8.DecodeRuneInString(f.Separator)
	if r == utf8.RuneError {
		return ','
	}
	return r
}

func (f Format) enc() rune {
	if f.Encloser == "" {
		return -1
	}
	r, _ := utf8.DecodeRuneInString(f.Encloser)
	return r
}

func (f Format) esc() rune {
	if f.Escape == "" {
		return f.enc()
	}
	r, _ := utf8.DecodeRuneInString(f.Escape)
	return r
}

// Reader parses records.
type Reader struct {
	r        *bufio.Reader
	f        Format
	sep, enc rune
	esc      rune
	line     int
	// BytesRead counts the bytes consumed (progress).
	BytesRead int64
}

type countReader struct {
	r io.Reader
	n *int64
}

func (c countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	*c.n += int64(n)
	return n, err
}

// NewReader wraps r according to f (encoding, BOM, skipped lines).
func NewReader(r io.Reader, f Format) (*Reader, error) {
	rd := &Reader{f: f, sep: f.sep(), enc: f.enc(), esc: f.esc()}
	var src io.Reader = countReader{r, &rd.BytesRead}
	if strings.EqualFold(f.Encoding, "latin1") || strings.EqualFold(f.Encoding, "windows-1252") {
		src = transform.NewReader(src, charmap.Windows1252.NewDecoder())
	}
	rd.r = bufio.NewReaderSize(src, 256<<10)
	if b, err := rd.r.Peek(3); err == nil && string(b) == "\xef\xbb\xbf" {
		_, _ = rd.r.Discard(3)
	}
	for i := 0; i < f.SkipLines; i++ {
		if _, err := rd.Read(); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
	}
	return rd, nil
}

// Line is the 1-based line where the last record started.
func (r *Reader) Line() int { return r.line }

// Read returns the next record; fields equal to the NULL marker (or empty,
// with EmptyNull) are nil. It returns io.EOF at the end.
func (r *Reader) Read() ([]*string, error) {
	var fields []*string
	var cur strings.Builder
	inQuotes, quoted, started := false, false, false
	r.line++
	startLine := r.line
	push := func() {
		v := cur.String()
		cur.Reset()
		if !quoted && (v == r.nullMarker() || (r.f.EmptyNull && v == "")) {
			fields = append(fields, nil)
		} else {
			fields = append(fields, &v)
		}
		quoted = false
	}
	for {
		ch, _, err := r.r.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if inQuotes {
					return nil, fmt.Errorf("linha %d: campo entre aspas não fechado", startLine)
				}
				if !started && len(fields) == 0 {
					return nil, io.EOF
				}
				push()
				return fields, nil
			}
			return nil, err
		}
		started = true
		if inQuotes {
			switch {
			case ch == r.esc && r.esc != r.enc:
				next, _, err := r.r.ReadRune()
				if err != nil {
					return nil, fmt.Errorf("linha %d: escape no fim do arquivo", startLine)
				}
				switch next {
				case 'n':
					cur.WriteByte('\n')
				case 't':
					cur.WriteByte('\t')
				case 'r':
					cur.WriteByte('\r')
				case '0':
					cur.WriteByte(0)
				default:
					cur.WriteRune(next)
				}
			case ch == r.enc:
				next, _, err := r.r.ReadRune()
				if err == nil && next == r.enc && r.esc == r.enc {
					cur.WriteRune(r.enc) // doubled encloser
					continue
				}
				inQuotes = false
				if err == nil {
					_ = r.r.UnreadRune()
				}
			default:
				if ch == '\n' {
					r.line++
				}
				cur.WriteRune(ch)
			}
			continue
		}
		switch {
		case ch == r.enc && cur.Len() == 0 && !quoted:
			inQuotes, quoted = true, true
		case ch == r.sep:
			push()
		case ch == '\r':
			// CRLF or lone CR end the record.
			if next, _, err := r.r.ReadRune(); err == nil && next != '\n' {
				_ = r.r.UnreadRune()
			}
			push()
			return fields, nil
		case ch == '\n':
			push()
			return fields, nil
		case ch == '\\' && r.esc == '\\':
			next, _, err := r.r.ReadRune()
			if err != nil {
				cur.WriteRune(ch)
				continue
			}
			if next == 'N' && cur.Len() == 0 {
				cur.WriteString(`\N`)
				continue
			}
			cur.WriteRune(next)
		default:
			cur.WriteRune(ch)
		}
	}
}

func (r *Reader) nullMarker() string {
	if r.f.NullMarker == "" {
		return `\N`
	}
	return r.f.NullMarker
}

// ---- detection ----------------------------------------------------------------------

// Detected is the result of scanning a file.
type Detected struct {
	Format  Format        `json:"format"`
	Header  []string      `json:"header"`
	Sample  [][]*string   `json:"sample"`
	Columns []ColumnGuess `json:"columns"`
}

// ColumnGuess is an inferred column type.
type ColumnGuess struct {
	Name     string `json:"name"`
	Type     string `json:"type"`   // e.g. INT, BIGINT, DECIMAL, DOUBLE, DATE, DATETIME, VARCHAR, TEXT
	Length   string `json:"length"` // "", "10,2", "255"
	Unsigned bool   `json:"unsigned"`
	Nullable bool   `json:"nullable"`
}

var (
	intRe      = regexp.MustCompile(`^-?\d{1,19}$`)
	decRe      = regexp.MustCompile(`^-?\d+[.,]\d+$`)
	floatRe    = regexp.MustCompile(`^-?(\d+\.?\d*|\.\d+)[eE][-+]?\d+$`)
	dateRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	datetimeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}(:\d{2}(\.\d{1,6})?)?$`)
	identRe    = regexp.MustCompile(`[^\p{L}\p{N}_]+`)
)

// Detect guesses separator, encloser and header from the beginning of a
// file, and infers column types from up to sampleRows records.
func Detect(head []byte, sampleRows int) Detected {
	text := string(head)
	// Drop a possibly cut last line.
	if i := strings.LastIndexAny(text, "\n"); i > 0 && len(head) >= 64<<10 {
		text = text[:i+1]
	}
	text = strings.TrimPrefix(text, "\ufeff")
	best := Format{Separator: ",", Encloser: `"`}
	bestScore := -1.0
	for _, sep := range []string{",", ";", "\t", "|"} {
		f := Format{Separator: sep, Encloser: `"`}
		recs := readAll(text, f, 50)
		if len(recs) == 0 {
			continue
		}
		counts := map[int]int{}
		for _, r := range recs {
			counts[len(r)]++
		}
		mode, freq := 0, 0
		for n, c := range counts {
			if c > freq || c == freq && n > mode {
				mode, freq = n, c
			}
		}
		if mode < 2 {
			continue
		}
		score := float64(freq)/float64(len(recs))*10 + math.Min(float64(mode), 50)/50
		if score > bestScore {
			best, bestScore = f, score
		}
	}
	if strings.Contains(text, `\"`) && !strings.Contains(text, `""`) {
		best.Escape = `\`
	}
	recs := readAll(text, best, sampleRows+1)
	d := Detected{Format: best}
	if len(recs) == 0 {
		return d
	}
	// Header: first row has no numeric field while the second has some, or
	// every first-row field looks like an identifier.
	first := recs[0]
	header := len(recs) > 1
	for _, v := range first {
		if v == nil || intRe.MatchString(*v) || decRe.MatchString(*v) || dateRe.MatchString(*v) {
			header = false
			break
		}
	}
	data := recs
	if header {
		d.Format.SkipLines = 1
		data = recs[1:]
		for i, v := range first {
			n := strings.Trim(identRe.ReplaceAllString(strings.TrimSpace(*v), "_"), "_")
			if n == "" {
				n = fmt.Sprintf("coluna%d", i+1)
			}
			d.Header = append(d.Header, n)
		}
	}
	cols := 0
	for _, r := range data {
		if len(r) > cols {
			cols = len(r)
		}
	}
	for len(d.Header) < cols {
		d.Header = append(d.Header, fmt.Sprintf("coluna%d", len(d.Header)+1))
	}
	for i := 0; i < cols; i++ {
		d.Columns = append(d.Columns, guess(d.Header[i], data, i))
	}
	if len(data) > 20 {
		d.Sample = data[:20]
	} else {
		d.Sample = data
	}
	return d
}

func readAll(text string, f Format, max int) [][]*string {
	r, err := NewReader(strings.NewReader(text), f)
	if err != nil {
		return nil
	}
	var out [][]*string
	for len(out) < max {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if len(rec) == 1 && rec[0] != nil && *rec[0] == "" {
			continue // blank line
		}
		out = append(out, rec)
	}
	return out
}

func guess(name string, rows [][]*string, i int) ColumnGuess {
	g := ColumnGuess{Name: name}
	isInt, isDec, isFloat, isDate, isDT := true, true, true, true, true
	maxLen, maxInt, minInt := 0, int64(0), int64(0)
	prec, scale := 0, 0
	seen := 0
	for _, r := range rows {
		if i >= len(r) || r[i] == nil || *r[i] == "" {
			g.Nullable = true
			continue
		}
		v := strings.TrimSpace(*r[i])
		seen++
		if l := utf8.RuneCountInString(v); l > maxLen {
			maxLen = l
		}
		if intRe.MatchString(v) {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				maxInt, minInt = max(maxInt, n), min(minInt, n)
			} else {
				isInt = false
			}
		} else {
			isInt = false
		}
		if decRe.MatchString(v) || intRe.MatchString(v) {
			parts := strings.FieldsFunc(strings.TrimPrefix(v, "-"), func(r rune) bool { return r == '.' || r == ',' })
			if len(parts) == 2 {
				scale = max(scale, len(parts[1]))
				prec = max(prec, len(parts[0]))
			} else if len(parts) == 1 {
				prec = max(prec, len(parts[0]))
			}
		} else {
			isDec = false
		}
		if !(floatRe.MatchString(v) || decRe.MatchString(v) || intRe.MatchString(v)) {
			isFloat = false
		}
		if !dateRe.MatchString(v) {
			isDate = false
		}
		if !datetimeRe.MatchString(v) {
			isDT = false
		}
	}
	switch {
	case seen == 0:
		g.Type, g.Length, g.Nullable = "VARCHAR", "255", true
	case isInt:
		g.Unsigned = minInt >= 0
		switch {
		case maxInt <= 127 && minInt >= -128:
			g.Type = "TINYINT"
		case maxInt <= 32767 && minInt >= -32768:
			g.Type = "SMALLINT"
		case maxInt <= 2147483647 && minInt >= -2147483648:
			g.Type = "INT"
		default:
			g.Type = "BIGINT"
		}
	case isDec && scale > 0 && prec+scale <= 65:
		g.Type, g.Length = "DECIMAL", fmt.Sprintf("%d,%d", prec+scale, scale)
	case isFloat:
		g.Type = "DOUBLE"
	case isDate:
		g.Type = "DATE"
	case isDT:
		g.Type = "DATETIME"
	case maxLen <= 255:
		n := ((maxLen + 9) / 10) * 10
		if n == 0 {
			n = 10
		}
		g.Type, g.Length = "VARCHAR", strconv.Itoa(n)
	case maxLen <= 65535/4:
		g.Type = "TEXT"
	default:
		g.Type = "MEDIUMTEXT"
	}
	return g
}

// NormalizeNumber converts "1.234,56" (local number format, HeidiSQL option)
// to "1234.56".
func NormalizeNumber(s, thousands, decimal string) string {
	if thousands != "" {
		s = strings.ReplaceAll(s, thousands, "")
	}
	if decimal != "" && decimal != "." {
		s = strings.ReplaceAll(s, decimal, ".")
	}
	return s
}
