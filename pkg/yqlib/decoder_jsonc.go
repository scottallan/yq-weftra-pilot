//go:build !yq_nojson

package yqlib

import (
	"fmt"
	"io"
	"strings"

	"github.com/goccy/go-json"
)

// jsoncDecoder decodes JSONC: standard JSON plus // and /* */ comments and a
// single tolerated trailing comma before a closing } or ].
//
// Comments are attached to the CandidateNode they annotate using the same
// HeadComment/LineComment/FootComment convention the YAML decoder already
// uses, following the closest analogous rule in each ambiguous case (see
// pkg/yqlib/doc/usage/headers/jsonc.md for the documented attachment rules):
//   - a comment on its own line immediately before a member/element is that
//     node's head comment (the key node, for object members),
//   - a comment trailing on the same line as a value (before/after a comma)
//     is that value's line comment,
//   - a comment on its own line after the last member/element, before the
//     closing bracket, is that last child's foot comment,
//   - a comment on the same line as a container's opening bracket (e.g.
//     `"dns": { // comment`) is treated the same as a comment on its own line
//     right after the opening bracket: the head comment of the first
//     member/element, or the container's own foot comment if it has no
//     children at all (a position YAML's flow parser silently drops the
//     comment in; we keep it rather than losing data).
type jsoncDecoder struct {
	parser          *jsoncParser
	documentIndex   uint
	pendingComments []string
	initialized     bool
}

func NewJSONCDecoder() Decoder {
	return &jsoncDecoder{}
}

func (dec *jsoncDecoder) Init(reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	parser, err := newJsoncParser(newJsoncLexer(string(data)))
	if err != nil {
		return err
	}
	dec.parser = parser
	dec.documentIndex = 0
	dec.pendingComments = nil
	dec.initialized = true
	return nil
}

func (dec *jsoncDecoder) Decode() (*CandidateNode, error) {
	if !dec.initialized {
		return nil, io.EOF
	}

	p := dec.parser

	leading, err := p.gatherComments()
	if err != nil {
		return nil, err
	}
	allLeading := append(dec.pendingComments, leading...)
	dec.pendingComments = nil

	if p.peek().typ == jsoncTokEOF {
		return nil, io.EOF
	}

	rootNode, endLine, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	rootNode.SetDocument(dec.documentIndex)
	dec.documentIndex++

	if len(allLeading) > 0 {
		rootNode.HeadComment = joinJsoncComments(allLeading)
	}

	// a comment on the same line as the last token of the document (e.g.
	// trailing after the closing brace) is this document's line comment.
	if p.peek().typ == jsoncTokComment && p.peek().line == endLine {
		tok, err := p.consume()
		if err != nil {
			return nil, err
		}
		rootNode.LineComment = appendJsoncComment(rootNode.LineComment, tok.raw)
	}

	// any further comments before the next document (or EOF) either become
	// this document's foot comment (nothing follows) or the next document's
	// head comment (handled at the top of the next Decode() call).
	trailing, err := p.gatherComments()
	if err != nil {
		return nil, err
	}
	if p.peek().typ == jsoncTokEOF {
		if len(trailing) > 0 {
			rootNode.FootComment = appendJsoncComment(rootNode.FootComment, joinJsoncComments(trailing))
		}
	} else {
		dec.pendingComments = trailing
	}

	return rootNode, nil
}

// jsoncParser is a small recursive-descent parser that builds a CandidateNode
// tree directly (rather than stripping comments/commas first and handing the
// result to the existing goccy-based JSON decoder), so that comment
// attachment can be tracked precisely. Scalar values are still interpreted
// via CandidateNode.setScalarFromJson so that number/bool/string/null
// semantics stay identical to the plain `json` format.
// jsoncMaxNestingDepth bounds recursive descent into nested objects/arrays so
// that a deeply (or maliciously) nested document returns a decode error
// instead of exhausting the Go call stack, matching the limit encoding/json
// applies for the same reason (see maxNestingDepth in encoding/json/scanner.go).
const jsoncMaxNestingDepth = 10000

type jsoncParser struct {
	lex   *jsoncLexer
	cur   jsoncToken
	depth int
}

func newJsoncParser(lex *jsoncLexer) (*jsoncParser, error) {
	p := &jsoncParser{lex: lex}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *jsoncParser) advance() error {
	tok, err := p.lex.Next()
	if err != nil {
		return err
	}
	p.cur = tok
	return nil
}

func (p *jsoncParser) peek() jsoncToken {
	return p.cur
}

func (p *jsoncParser) consume() (jsoncToken, error) {
	tok := p.cur
	if err := p.advance(); err != nil {
		return jsoncToken{}, err
	}
	return tok, nil
}

// gatherComments consumes and returns every consecutive comment token
// starting at the current position.
func (p *jsoncParser) gatherComments() ([]string, error) {
	var comments []string
	for p.peek().typ == jsoncTokComment {
		tok, err := p.consume()
		if err != nil {
			return nil, err
		}
		comments = append(comments, tok.raw)
	}
	return comments, nil
}

func (p *jsoncParser) parseValue() (*CandidateNode, int, error) {
	switch p.peek().typ {
	case jsoncTokLBrace:
		return p.parseObject()
	case jsoncTokLBracket:
		return p.parseArray()
	case jsoncTokString, jsoncTokLiteral:
		return p.parseScalar()
	case jsoncTokEOF:
		return nil, 0, fmt.Errorf("line %d: unexpected end of input, expected a value", p.peek().line)
	default:
		return nil, 0, fmt.Errorf("line %d: unexpected %q, expected a value", p.peek().line, p.peek().raw)
	}
}

func (p *jsoncParser) parseScalar() (*CandidateNode, int, error) {
	tok, err := p.consume()
	if err != nil {
		return nil, 0, err
	}
	var value interface{}
	if err := json.Unmarshal([]byte(tok.raw), &value); err != nil {
		return nil, 0, fmt.Errorf("line %d: invalid value %q: %w", tok.line, tok.raw, err)
	}
	node := &CandidateNode{Kind: ScalarNode, Line: tok.line}
	if err := node.setScalarFromJson(value); err != nil {
		return nil, 0, err
	}
	return node, tok.line, nil
}

func (p *jsoncParser) parseObject() (*CandidateNode, int, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > jsoncMaxNestingDepth {
		return nil, 0, fmt.Errorf("line %d: exceeded max depth of %d", p.peek().line, jsoncMaxNestingDepth)
	}

	node := &CandidateNode{Kind: MappingNode, Tag: "!!map"}
	openTok, err := p.consume() // '{'
	if err != nil {
		return nil, 0, err
	}
	node.Line = openTok.line

	var content []*CandidateNode
	sawComma := true // no previous member yet, so nothing requires a separating comma
	for {
		headComments, err := p.gatherComments()
		if err != nil {
			return nil, 0, err
		}
		if p.peek().typ == jsoncTokRBrace {
			closeTok, err := p.consume()
			if err != nil {
				return nil, 0, err
			}
			attachJsoncFootComments(content, headComments, node, false)
			node.Content = content
			return node, closeTok.line, nil
		}
		if !sawComma {
			return nil, 0, fmt.Errorf("line %d: expected ',' or '}', got %q", p.peek().line, p.peek().raw)
		}

		if p.peek().typ != jsoncTokString {
			return nil, 0, fmt.Errorf("line %d: expected a string key or '}', got %q", p.peek().line, p.peek().raw)
		}
		keyTok, err := p.consume()
		if err != nil {
			return nil, 0, err
		}
		var keyValue string
		if err := json.Unmarshal([]byte(keyTok.raw), &keyValue); err != nil {
			return nil, 0, fmt.Errorf("line %d: invalid object key %q: %w", keyTok.line, keyTok.raw, err)
		}
		keyNode := &CandidateNode{Parent: node, Kind: ScalarNode, Tag: "!!str", Value: keyValue, IsMapKey: true, Line: keyTok.line}

		if p.peek().typ != jsoncTokColon {
			return nil, 0, fmt.Errorf("line %d: expected ':' after object key, got %q", p.peek().line, p.peek().raw)
		}
		if _, err := p.consume(); err != nil {
			return nil, 0, err
		}

		moreComments, err := p.gatherComments()
		if err != nil {
			return nil, 0, err
		}
		keyNode.HeadComment = joinJsoncComments(append(headComments, moreComments...))

		valueNode, valueEndLine, err := p.parseValue()
		if err != nil {
			return nil, 0, err
		}
		valueNode.Parent = node
		valueNode.Key = keyNode

		lastLine := valueEndLine
		sawComma = false
		if p.peek().typ == jsoncTokComma {
			commaTok, err := p.consume()
			if err != nil {
				return nil, 0, err
			}
			sawComma = true
			lastLine = commaTok.line
		}
		if p.peek().typ == jsoncTokComment && p.peek().line == lastLine {
			tok, err := p.consume()
			if err != nil {
				return nil, 0, err
			}
			valueNode.LineComment = appendJsoncComment(valueNode.LineComment, tok.raw)
		}

		content = append(content, keyNode, valueNode)
	}
}

func (p *jsoncParser) parseArray() (*CandidateNode, int, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > jsoncMaxNestingDepth {
		return nil, 0, fmt.Errorf("line %d: exceeded max depth of %d", p.peek().line, jsoncMaxNestingDepth)
	}

	node := &CandidateNode{Kind: SequenceNode, Tag: "!!seq"}
	openTok, err := p.consume() // '['
	if err != nil {
		return nil, 0, err
	}
	node.Line = openTok.line

	var content []*CandidateNode
	idx := 0
	sawComma := true // no previous element yet, so nothing requires a separating comma
	for {
		headComments, err := p.gatherComments()
		if err != nil {
			return nil, 0, err
		}
		if p.peek().typ == jsoncTokRBracket {
			closeTok, err := p.consume()
			if err != nil {
				return nil, 0, err
			}
			attachJsoncFootComments(content, headComments, node, true)
			node.Content = content
			return node, closeTok.line, nil
		}
		if !sawComma {
			return nil, 0, fmt.Errorf("line %d: expected ',' or ']', got %q", p.peek().line, p.peek().raw)
		}

		elemNode, elemEndLine, err := p.parseValue()
		if err != nil {
			return nil, 0, err
		}
		elemNode.Parent = node
		elemNode.HeadComment = joinJsoncComments(headComments)
		keyNode := &CandidateNode{Parent: node, Kind: ScalarNode, Tag: "!!int", Value: fmt.Sprintf("%v", idx), IsMapKey: true}
		elemNode.Key = keyNode
		idx++

		lastLine := elemEndLine
		sawComma = false
		if p.peek().typ == jsoncTokComma {
			commaTok, err := p.consume()
			if err != nil {
				return nil, 0, err
			}
			sawComma = true
			lastLine = commaTok.line
		}
		if p.peek().typ == jsoncTokComment && p.peek().line == lastLine {
			tok, err := p.consume()
			if err != nil {
				return nil, 0, err
			}
			elemNode.LineComment = appendJsoncComment(elemNode.LineComment, tok.raw)
		}

		content = append(content, elemNode)
	}
}

func attachJsoncFootComments(content []*CandidateNode, comments []string, container *CandidateNode, isArray bool) {
	if len(comments) == 0 {
		return
	}
	text := joinJsoncComments(comments)
	if len(content) == 0 {
		container.FootComment = appendJsoncComment(container.FootComment, text)
		return
	}
	var target *CandidateNode
	if isArray {
		target = content[len(content)-1]
	} else {
		target = content[len(content)-2] // the last key node
	}
	target.FootComment = appendJsoncComment(target.FootComment, text)
}

func joinJsoncComments(comments []string) string {
	return strings.Join(comments, "\n")
}

func appendJsoncComment(existing string, addition string) string {
	if existing == "" {
		return addition
	}
	if addition == "" {
		return existing
	}
	return existing + "\n" + addition
}
