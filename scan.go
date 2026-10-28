package kulynflags

import "strings"

// defaultScanFunctions are used by FindFlagKeys when no function names are given.
var defaultScanFunctions = []string{"IsEnabled", "GetFlag", "Flag", "Evaluate"}

// maxScanKeyLength is the upper bound for a key literal. Longer literals are ignored.
const maxScanKeyLength = 100

// FindFlagKeys returns the flag keys that are passed as double-quoted string literals
// to calls of the given functions in Go source text, for example IsEnabled("checkout.new-ui").
//
// If no function names are given, IsEnabled, GetFlag, Flag and Evaluate are used.
// Method calls such as client.IsEnabled("x") are found too, because only the function
// name is matched.
//
// The scanner is a lexer, not a parser. It recognizes these cases:
//   - Comments (// and /* */) are skipped.
//   - Raw string literals (backticks) are skipped. Keys in them are not returned.
//   - Rune literals ('x') are skipped, so a quote inside them does not start a string.
//   - Text inside string literals is never read as code.
//   - A key must be the whole argument: IsEnabled("a" + b) is not a match.
//   - Keys with escape sequences, empty keys and keys over 100 bytes are ignored.
//
// Keys are not validated here. Invalid-looking literals are returned as they are, so that
// CheckHygiene can report them. Dynamic keys (variables, concatenation, fmt.Sprintf) are not
// detected. The function does not read files. The caller passes the source text.
//
// The run time is linear in the length of source.
func FindFlagKeys(source string, functions ...string) map[string]struct{} {
	names := functions
	if len(names) == 0 {
		names = defaultScanFunctions
	}
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[name] = struct{}{}
	}

	keys := make(map[string]struct{})
	lx := lexer{src: source}
	state := scanIdle
	var pending string

	for tok, ok := lx.next(); ok; tok, ok = lx.next() {
		next := scanIdle
		switch state {
		case scanAfterName:
			if tok.kind == tokPunct && tok.text == "(" {
				next = scanAfterOpen
			}
		case scanAfterOpen:
			if tok.kind == tokString && tok.valid {
				pending = tok.text
				next = scanAfterKey
			}
		case scanAfterKey:
			if tok.kind == tokPunct && (tok.text == "," || tok.text == ")") {
				keys[pending] = struct{}{}
			}
		}
		// A wanted name always starts a new candidate, even in the middle of another one.
		if tok.kind == tokIdent {
			if _, ok := wanted[tok.text]; ok {
				next = scanAfterName
			}
		}
		state = next
	}
	return keys
}

type scanState uint8

const (
	scanIdle      scanState = iota
	scanAfterName           // saw a wanted function name
	scanAfterOpen           // saw "(" after the name
	scanAfterKey            // saw a string literal as first argument
)

type tokenKind uint8

const (
	tokOther  tokenKind = iota // numbers, runes, raw strings
	tokIdent                   // identifiers and keywords
	tokString                  // double-quoted string literal
	tokPunct                   // single punctuation byte
)

type token struct {
	kind tokenKind
	// text is the identifier, the string content (without quotes) or the punctuation byte.
	text string
	// valid is set for string literals that can be used as a key: no escapes, 1 to 100 bytes.
	valid bool
}

// lexer splits Go source into tokens. Each byte is visited a constant number of times,
// so the run time is linear.
type lexer struct {
	src string
	pos int
}

// next returns the next token, or false at the end of the input.
func (l *lexer) next() (token, bool) {
	s := l.src
	for l.pos < len(s) {
		c := s[l.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v':
			l.pos++

		case c == '/' && l.at(l.pos+1) == '/':
			end := strings.IndexByte(s[l.pos:], '\n')
			if end == -1 {
				l.pos = len(s)
			} else {
				l.pos += end
			}

		case c == '/' && l.at(l.pos+1) == '*':
			end := strings.Index(s[l.pos+2:], "*/")
			if end == -1 {
				l.pos = len(s)
			} else {
				l.pos += 2 + end + 2
			}

		case c == '"':
			return l.quoted('"'), true

		case c == '\'':
			tok := l.quoted('\'')
			tok.kind = tokOther
			return tok, true

		case c == '`':
			end := strings.IndexByte(s[l.pos+1:], '`')
			if end == -1 {
				l.pos = len(s)
			} else {
				l.pos += 1 + end + 1
			}
			return token{kind: tokOther}, true

		case isIdentStart(c):
			start := l.pos
			for l.pos < len(s) && isIdentPart(s[l.pos]) {
				l.pos++
			}
			return token{kind: tokIdent, text: s[start:l.pos]}, true

		case isDigit(c):
			for l.pos < len(s) && isIdentPart(s[l.pos]) {
				l.pos++
			}
			return token{kind: tokOther}, true

		default:
			l.pos++
			return token{kind: tokPunct, text: s[l.pos-1 : l.pos]}, true
		}
	}
	return token{}, false
}

// quoted reads a quoted literal that starts at l.pos. An unterminated literal ends at the
// end of its line, so that one broken string does not hide the rest of the file.
func (l *lexer) quoted(quote byte) token {
	s := l.src
	start := l.pos + 1
	i := start
	for i < len(s) {
		c := s[i]
		if c == '\\' {
			if i+1 < len(s) && s[i+1] == '\n' {
				break
			}
			i += 2
			continue
		}
		if c == quote {
			text := s[start:i]
			l.pos = i + 1
			return token{
				kind:  tokString,
				text:  text,
				valid: quote == '"' && text != "" && len(text) <= maxScanKeyLength && !strings.Contains(text, "\\"),
			}
		}
		if c == '\n' {
			break
		}
		i++
	}
	if i > len(s) {
		i = len(s)
	}
	l.pos = i
	return token{kind: tokOther}
}

// at returns the byte at index i, or 0 when i is past the end.
func (l *lexer) at(i int) byte {
	if i >= len(l.src) {
		return 0
	}
	return l.src[i]
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c >= 0x80
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}
