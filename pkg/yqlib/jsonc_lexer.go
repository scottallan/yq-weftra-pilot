//go:build !yq_nojson

package yqlib

import (
	"fmt"
	"strings"
)

// jsoncTokenType enumerates the tokens produced when lexing JSONC (JSON plus
// // and /* */ comments, plus a single tolerated trailing comma).
type jsoncTokenType int

const (
	jsoncTokEOF jsoncTokenType = iota
	jsoncTokLBrace
	jsoncTokRBrace
	jsoncTokLBracket
	jsoncTokRBracket
	jsoncTokColon
	jsoncTokComma
	jsoncTokString  // raw includes the surrounding quotes, ready for json.Unmarshal
	jsoncTokLiteral // a bareword run: number, true, false or null
	jsoncTokComment // raw is the comment text with delimiters stripped and trimmed
)

type jsoncToken struct {
	typ  jsoncTokenType
	raw  string
	line int // 1-based line on which the token starts
}

// jsoncLexer is a hand-rolled scanner: it exists because comments and a
// tolerated trailing comma are not valid JSON, so the standard library/goccy
// JSON tokenizers cannot be driven directly over jsonc source.
type jsoncLexer struct {
	src  string
	pos  int
	line int
}

func newJsoncLexer(src string) *jsoncLexer {
	return &jsoncLexer{src: src, pos: 0, line: 1}
}

func (l *jsoncLexer) advanceByte() byte {
	b := l.src[l.pos]
	l.pos++
	if b == '\n' {
		l.line++
	}
	return b
}

func isJsoncLiteralChar(b byte) bool {
	switch {
	case b >= '0' && b <= '9':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b == '-' || b == '+' || b == '.':
		return true
	}
	return false
}

// Next returns the next token in the stream, skipping insignificant
// whitespace. Comments are returned as first-class tokens (rather than
// skipped) so the parser can decide where they attach.
func (l *jsoncLexer) Next() (jsoncToken, error) {
	for l.pos < len(l.src) {
		b := l.src[l.pos]
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			l.advanceByte()
			continue
		}
		break
	}
	if l.pos >= len(l.src) {
		return jsoncToken{typ: jsoncTokEOF, line: l.line}, nil
	}

	startLine := l.line
	b := l.src[l.pos]
	switch b {
	case '{':
		l.advanceByte()
		return jsoncToken{jsoncTokLBrace, "{", startLine}, nil
	case '}':
		l.advanceByte()
		return jsoncToken{jsoncTokRBrace, "}", startLine}, nil
	case '[':
		l.advanceByte()
		return jsoncToken{jsoncTokLBracket, "[", startLine}, nil
	case ']':
		l.advanceByte()
		return jsoncToken{jsoncTokRBracket, "]", startLine}, nil
	case ':':
		l.advanceByte()
		return jsoncToken{jsoncTokColon, ":", startLine}, nil
	case ',':
		l.advanceByte()
		return jsoncToken{jsoncTokComma, ",", startLine}, nil
	case '"':
		return l.lexString(startLine)
	case '/':
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			return l.lexLineComment(startLine)
		}
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '*' {
			return l.lexBlockComment(startLine)
		}
		return jsoncToken{}, fmt.Errorf("line %d: unexpected character '/'", startLine)
	default:
		if isJsoncLiteralChar(b) {
			return l.lexLiteral(startLine), nil
		}
		return jsoncToken{}, fmt.Errorf("line %d: unexpected character %q", startLine, string(rune(b)))
	}
}

func (l *jsoncLexer) lexString(startLine int) (jsoncToken, error) {
	start := l.pos
	l.advanceByte() // opening quote
	for {
		if l.pos >= len(l.src) {
			return jsoncToken{}, fmt.Errorf("line %d: unterminated string literal", startLine)
		}
		b := l.src[l.pos]
		if b == '\\' {
			l.advanceByte()
			if l.pos >= len(l.src) {
				return jsoncToken{}, fmt.Errorf("line %d: unterminated string literal", startLine)
			}
			l.advanceByte() // consume the escaped character; \uXXXX's hex digits fall through the loop as normal characters
			continue
		}
		if b == '"' {
			l.advanceByte()
			break
		}
		if b == '\n' {
			return jsoncToken{}, fmt.Errorf("line %d: unterminated string literal", startLine)
		}
		l.advanceByte()
	}
	return jsoncToken{jsoncTokString, l.src[start:l.pos], startLine}, nil
}

func (l *jsoncLexer) lexLiteral(startLine int) jsoncToken {
	start := l.pos
	for l.pos < len(l.src) && isJsoncLiteralChar(l.src[l.pos]) {
		l.advanceByte()
	}
	return jsoncToken{jsoncTokLiteral, l.src[start:l.pos], startLine}
}

func (l *jsoncLexer) lexLineComment(startLine int) (jsoncToken, error) {
	l.advanceByte()
	l.advanceByte() // consume '//'
	start := l.pos
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.advanceByte()
	}
	return jsoncToken{jsoncTokComment, strings.TrimSpace(l.src[start:l.pos]), startLine}, nil
}

func (l *jsoncLexer) lexBlockComment(startLine int) (jsoncToken, error) {
	l.advanceByte()
	l.advanceByte() // consume '/*'
	start := l.pos
	for {
		if l.pos >= len(l.src) {
			return jsoncToken{}, fmt.Errorf("line %d: unterminated block comment", startLine)
		}
		if l.src[l.pos] == '*' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/' {
			break
		}
		l.advanceByte()
	}
	raw := l.src[start:l.pos]
	l.advanceByte()
	l.advanceByte() // consume '*/'
	return jsoncToken{jsoncTokComment, normalizeJsoncBlockComment(raw), startLine}, nil
}

// normalizeJsoncBlockComment trims each line of a /* ... */ comment and drops
// blank lines that only exist because of where the delimiters were placed
// (e.g. "/*\nfoo\n*/"), so the encoder can re-emit the comment as one or more
// tidy "// " lines.
func normalizeJsoncBlockComment(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimSpace(ln)
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}
