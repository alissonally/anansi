// Package sqllex implements a tolerant tokenizer for the MySQL/MariaDB SQL
// dialect. It never fails: unterminated strings, quoted identifiers and
// comments simply run until the end of the input, which is what an editor
// needs while the user is still typing.
//
// The tokenizer is the foundation for statement splitting, statement
// classification and context-aware autocompletion. It deliberately does not
// try to build a full syntax tree.
package sqllex

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind identifies the lexical category of a token.
type Kind int

const (
	Whitespace Kind = iota
	Comment
	Word        // keyword or unquoted identifier
	QuotedIdent // `identifier`
	String      // 'text' or "text" (ANSI_QUOTES is not assumed)
	Number      // 123, 1.5, 0x1F, 1e10
	Operator    // =, <>, +, ...
	Punct       // ( ) , ;
	Dot         // .
	Variable    // @var, @@session.var
	Placeholder // ?
	// Conditional is an executable comment (/*! ... */, /*M! ... */): MySQL
	// and MariaDB run its content, so it is significant, not a comment.
	Conditional
)

// Token is a lexical unit. Start and End are byte offsets into the source.
type Token struct {
	Kind  Kind
	Text  string
	Start int
	End   int
	// Open reports that a string, quoted identifier or block comment was not
	// closed before the end of the input.
	Open bool
}

// Upper returns the token text upper-cased; useful for keyword comparisons.
func (t Token) Upper() string { return strings.ToUpper(t.Text) }

// IsKeyword reports whether the token is a bare word equal (case-insensitive)
// to any of the given keywords.
func (t Token) IsKeyword(kw ...string) bool {
	if t.Kind != Word {
		return false
	}
	for _, k := range kw {
		if strings.EqualFold(t.Text, k) {
			return true
		}
	}
	return false
}

// Ident returns the identifier value of a Word or QuotedIdent token with
// quoting removed and doubled backticks unescaped.
func (t Token) Ident() string {
	if t.Kind != QuotedIdent {
		return t.Text
	}
	s := t.Text
	if len(s) > 0 && s[0] == '`' {
		s = s[1:]
	}
	if !t.Open && len(s) > 0 && s[len(s)-1] == '`' {
		s = s[:len(s)-1]
	}
	return strings.ReplaceAll(s, "``", "`")
}

// Significant reports whether the token carries syntax (not whitespace or a
// comment).
func (t Token) Significant() bool { return t.Kind != Whitespace && t.Kind != Comment }

// Tokenize splits src into tokens. Concatenating the Text of all tokens
// reproduces src exactly.
func Tokenize(src string) []Token {
	var toks []Token
	i := 0
	n := len(src)
	for i < n {
		start := i
		c := src[i]
		switch {
		case isSpace(c):
			for i < n && isSpace(src[i]) {
				i++
			}
			toks = append(toks, Token{Kind: Whitespace, Text: src[start:i], Start: start, End: i})
		case c == '#' || (c == '-' && i+1 < n && src[i+1] == '-' && (i+2 >= n || isSpace(src[i+2]))):
			for i < n && src[i] != '\n' {
				i++
			}
			toks = append(toks, Token{Kind: Comment, Text: src[start:i], Start: start, End: i})
		case c == '/' && i+1 < n && src[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			open := end < 0
			if open {
				i = n
			} else {
				i = i + 2 + end + 2
			}
			kind := Comment
			if strings.HasPrefix(src[start:], "/*!") || strings.HasPrefix(src[start:], "/*M!") {
				kind = Conditional
			}
			toks = append(toks, Token{Kind: kind, Text: src[start:i], Start: start, End: i, Open: open})
		case c == '\'' || c == '"':
			j, open := scanQuoted(src, i, c, true)
			i = j
			toks = append(toks, Token{Kind: String, Text: src[start:i], Start: start, End: i, Open: open})
		case c == '`':
			j, open := scanQuoted(src, i, '`', false)
			i = j
			toks = append(toks, Token{Kind: QuotedIdent, Text: src[start:i], Start: start, End: i, Open: open})
		case isDigit(c) || (c == '.' && i+1 < n && isDigit(src[i+1]) && !prevIsIdentLike(toks)):
			i = scanNumber(src, i)
			// A number immediately followed by identifier characters is an
			// identifier in MySQL (e.g. 1abc), keep scanning as a word.
			if i < n && isIdentByte(src, i) && !(src[start] == '0' && i-start == 1) {
				for i < n && isIdentByte(src, i) {
					_, sz := utf8.DecodeRuneInString(src[i:])
					i += sz
				}
				toks = append(toks, Token{Kind: Word, Text: src[start:i], Start: start, End: i})
			} else {
				toks = append(toks, Token{Kind: Number, Text: src[start:i], Start: start, End: i})
			}
		case c == '@':
			i++
			if i < n && src[i] == '@' {
				i++
			}
			if i < n && (src[i] == '`' || src[i] == '\'' || src[i] == '"') {
				i, _ = scanQuoted(src, i, src[i], src[i] != '`')
			} else {
				for i < n && (isIdentByte(src, i) || src[i] == '.') {
					_, sz := utf8.DecodeRuneInString(src[i:])
					i += sz
				}
			}
			toks = append(toks, Token{Kind: Variable, Text: src[start:i], Start: start, End: i})
		case isIdentByte(src, i):
			for i < n && isIdentByte(src, i) {
				_, sz := utf8.DecodeRuneInString(src[i:])
				i += sz
			}
			toks = append(toks, Token{Kind: Word, Text: src[start:i], Start: start, End: i})
		case c == '.':
			i++
			toks = append(toks, Token{Kind: Dot, Text: ".", Start: start, End: i})
		case c == '(' || c == ')' || c == ',' || c == ';':
			i++
			toks = append(toks, Token{Kind: Punct, Text: src[start:i], Start: start, End: i})
		case c == '?':
			i++
			toks = append(toks, Token{Kind: Placeholder, Text: "?", Start: start, End: i})
		default:
			i = scanOperator(src, i)
			toks = append(toks, Token{Kind: Operator, Text: src[start:i], Start: start, End: i})
		}
	}
	return toks
}

func scanQuoted(src string, i int, q byte, backslash bool) (int, bool) {
	n := len(src)
	i++ // opening quote
	for i < n {
		c := src[i]
		if backslash && c == '\\' {
			i += 2
			continue
		}
		if c == q {
			if i+1 < n && src[i+1] == q { // doubled quote escape
				i += 2
				continue
			}
			return i + 1, false
		}
		i++
	}
	return n, true
}

func scanNumber(src string, i int) int {
	n := len(src)
	if src[i] == '0' && i+1 < n && (src[i+1] == 'x' || src[i+1] == 'X' || src[i+1] == 'b' || src[i+1] == 'B') {
		j := i + 2
		for j < n && isHex(src[j]) {
			j++
		}
		if j > i+2 {
			return j
		}
	}
	for i < n && isDigit(src[i]) {
		i++
	}
	if i < n && src[i] == '.' {
		i++
		for i < n && isDigit(src[i]) {
			i++
		}
	}
	if i < n && (src[i] == 'e' || src[i] == 'E') {
		j := i + 1
		if j < n && (src[j] == '+' || src[j] == '-') {
			j++
		}
		if j < n && isDigit(src[j]) {
			for j < n && isDigit(src[j]) {
				j++
			}
			i = j
		}
	}
	return i
}

func scanOperator(src string, i int) int {
	n := len(src)
	for _, op := range []string{"<=>", "<<", ">>", "<=", ">=", "<>", "!=", ":=", "&&", "||", "->>", "->"} {
		if strings.HasPrefix(src[i:], op) {
			return i + len(op)
		}
	}
	_, sz := utf8.DecodeRuneInString(src[i:])
	if i+sz > n {
		return n
	}
	return i + sz
}

func prevIsIdentLike(toks []Token) bool {
	if len(toks) == 0 {
		return false
	}
	k := toks[len(toks)-1].Kind
	return k == Word || k == QuotedIdent
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isIdentByte(src string, i int) bool {
	c := src[i]
	if c < utf8.RuneSelf {
		return c == '_' || c == '$' || isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	r, _ := utf8.DecodeRuneInString(src[i:])
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// ConditionalBody returns the SQL inside an executable comment, without the
// marker and version number: "/*!40101 SET x=1 */" → "SET x=1".
func ConditionalBody(text string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(text, "/*M!"), "/*!")
	s = strings.TrimSuffix(s, "*/")
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return strings.TrimSpace(s[i:])
}
