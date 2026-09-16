//go:build !yq_nojson

package yqlib

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/goccy/go-json"
)

// jsoncEncoder writes a CandidateNode tree back out as JSONC: structurally
// identical to the `json` encoder's own number/string/formatting rules, plus
// any HeadComment/LineComment/FootComment re-emitted as `//` comments at the
// position they were decoded from. It never emits a trailing comma -
// trailing-comma tolerance is input-only.
type jsoncEncoder struct {
	prefs JsonPreferences
}

func NewJSONCEncoder(prefs JsonPreferences) Encoder {
	return &jsoncEncoder{prefs}
}

func (je *jsoncEncoder) CanHandleAliases() bool {
	return false
}

func (je *jsoncEncoder) PrintDocumentSeparator(_ io.Writer) error {
	return nil
}

func (je *jsoncEncoder) PrintLeadingContent(_ io.Writer, _ string) error {
	return nil
}

func (je *jsoncEncoder) Encode(writer io.Writer, node *CandidateNode) error {
	if node.Kind == ScalarNode && je.prefs.UnwrapScalar {
		return encodeUnwrappedJsoncScalar(writer, node)
	}

	// A single trailing comma tolerance aside, jsonc is structurally JSON: an
	// indent of 0 means fully compact, single-line output, exactly like
	// `-o json`. That is incompatible with `//` comments, which run to the
	// end of a physical line, so whenever the tree carries a comment we fall
	// back to an indented layout to avoid producing unparsable output.
	indent := je.prefs.Indent
	if indent <= 0 && hasAnyJsoncComment(node, 0) {
		indent = 2
	}
	ctx := jsoncWriteCtx{indentStr: strings.Repeat(" ", indent), pretty: indent > 0}

	var buf bytes.Buffer
	writeJsoncCommentLines(&buf, node.HeadComment, ctx, 0)
	if err := ctx.writeValue(&buf, node, 0); err != nil {
		return err
	}
	if node.LineComment != "" {
		buf.WriteString(" // ")
		buf.WriteString(singleLineJsoncComment(node.LineComment))
	}
	buf.WriteString("\n")
	writeJsoncCommentLines(&buf, node.FootComment, ctx, 0)

	if je.prefs.ColorsEnabled {
		return colorizeAndPrint(buf.Bytes(), writer)
	}
	return writeString(writer, buf.String())
}

func encodeUnwrappedJsoncScalar(writer io.Writer, node *CandidateNode) error {
	var buf bytes.Buffer
	for _, l := range splitJsoncCommentLines(node.HeadComment) {
		buf.WriteString("// ")
		buf.WriteString(l)
		buf.WriteString("\n")
	}
	buf.WriteString(node.Value)
	if node.LineComment != "" {
		buf.WriteString(" // ")
		buf.WriteString(singleLineJsoncComment(node.LineComment))
	}
	buf.WriteString("\n")
	for _, l := range splitJsoncCommentLines(node.FootComment) {
		buf.WriteString("// ")
		buf.WriteString(l)
		buf.WriteString("\n")
	}
	return writeString(writer, buf.String())
}

type jsoncWriteCtx struct {
	indentStr string
	pretty    bool
}

func (ctx jsoncWriteCtx) writeIndent(buf *bytes.Buffer, depth int) {
	for i := 0; i < depth; i++ {
		buf.WriteString(ctx.indentStr)
	}
}

func (ctx jsoncWriteCtx) writeValue(buf *bytes.Buffer, node *CandidateNode, depth int) error {
	// mirrors jsoncMaxNestingDepth in decoder_jsonc.go: bounds recursion so a
	// tree with attacker-controlled nesting (e.g. round-tripped from another
	// format's decoder) can't exhaust the Go call stack.
	if depth > jsoncMaxNestingDepth {
		return fmt.Errorf("exceeded max depth of %d", jsoncMaxNestingDepth)
	}
	switch node.Kind {
	case MappingNode:
		return ctx.writeMapping(buf, node, depth)
	case SequenceNode:
		return ctx.writeSequence(buf, node, depth)
	case ScalarNode:
		return ctx.writeScalar(buf, node)
	case AliasNode:
		return ctx.writeValue(buf, node.Alias, depth)
	default:
		buf.WriteString("null")
		return nil
	}
}

func (ctx jsoncWriteCtx) writeMapping(buf *bytes.Buffer, node *CandidateNode, depth int) error {
	if len(node.Content) == 0 {
		buf.WriteString("{}")
		return nil
	}
	buf.WriteString("{")
	for i := 0; i < len(node.Content); i += 2 {
		keyNode, valueNode := node.Content[i], node.Content[i+1]
		if ctx.pretty {
			buf.WriteString("\n")
			writeJsoncCommentLines(buf, keyNode.HeadComment, ctx, depth+1)
			ctx.writeIndent(buf, depth+1)
		}
		keyBytes, err := jsoncStringBytes(keyNode.Value)
		if err != nil {
			return err
		}
		buf.Write(keyBytes)
		buf.WriteString(":")
		if ctx.pretty {
			buf.WriteString(" ")
		}
		if err := ctx.writeValue(buf, valueNode, depth+1); err != nil {
			return err
		}
		isLast := i == len(node.Content)-2
		if !isLast {
			buf.WriteString(",")
		}
		if valueNode.LineComment != "" {
			buf.WriteString(" // ")
			buf.WriteString(singleLineJsoncComment(valueNode.LineComment))
		}
		if isLast && ctx.pretty {
			buf.WriteString("\n")
			writeJsoncCommentLines(buf, keyNode.FootComment, ctx, depth+1)
			ctx.writeIndent(buf, depth)
		}
	}
	buf.WriteString("}")
	return nil
}

func (ctx jsoncWriteCtx) writeSequence(buf *bytes.Buffer, node *CandidateNode, depth int) error {
	if len(node.Content) == 0 {
		buf.WriteString("[]")
		return nil
	}
	buf.WriteString("[")
	for i, child := range node.Content {
		if ctx.pretty {
			buf.WriteString("\n")
			writeJsoncCommentLines(buf, child.HeadComment, ctx, depth+1)
			ctx.writeIndent(buf, depth+1)
		}
		if err := ctx.writeValue(buf, child, depth+1); err != nil {
			return err
		}
		isLast := i == len(node.Content)-1
		if !isLast {
			buf.WriteString(",")
		}
		if child.LineComment != "" {
			buf.WriteString(" // ")
			buf.WriteString(singleLineJsoncComment(child.LineComment))
		}
		if isLast && ctx.pretty {
			buf.WriteString("\n")
			writeJsoncCommentLines(buf, child.FootComment, ctx, depth+1)
			ctx.writeIndent(buf, depth)
		}
	}
	buf.WriteString("]")
	return nil
}

func (ctx jsoncWriteCtx) writeScalar(buf *bytes.Buffer, node *CandidateNode) error {
	data, err := scalarJsoncBytes(node)
	if err != nil {
		return err
	}
	buf.Write(data)
	return nil
}

// scalarJsoncBytes mirrors CandidateNode.MarshalJSON's ScalarNode case
// exactly (including the float-literal preservation), minus the trailing
// newline a json.Encoder always appends - that newline is fine when the
// caller compacts the embedded bytes (as encoding/json does for a
// json.Marshaler), but this encoder writes bytes directly into the output
// stream, where a stray newline would corrupt the structure.
func scalarJsoncBytes(node *CandidateNode) ([]byte, error) {
	if node.guessTagFromCustomType() == "!!float" {
		if raw, ok := jsonFloatLiteral(node.Value); ok {
			return []byte(raw), nil
		}
	}
	value, err := node.GetValueRep()
	if err != nil {
		return nil, err
	}
	return jsoncMarshal(value)
}

func jsoncStringBytes(s string) ([]byte, error) {
	return jsoncMarshal(s)
}

func jsoncMarshal(value interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func hasAnyJsoncComment(node *CandidateNode, depth int) bool {
	if node == nil || depth > jsoncMaxNestingDepth {
		return false
	}
	if node.HeadComment != "" || node.LineComment != "" || node.FootComment != "" {
		return true
	}
	for _, c := range node.Content {
		if hasAnyJsoncComment(c, depth+1) {
			return true
		}
	}
	return false
}

func writeJsoncCommentLines(buf *bytes.Buffer, comment string, ctx jsoncWriteCtx, depth int) {
	for _, l := range splitJsoncCommentLines(comment) {
		ctx.writeIndent(buf, depth)
		buf.WriteString("// ")
		buf.WriteString(l)
		buf.WriteString("\n")
	}
}

func splitJsoncCommentLines(comment string) []string {
	if comment == "" {
		return nil
	}
	return strings.Split(comment, "\n")
}

func singleLineJsoncComment(comment string) string {
	return strings.ReplaceAll(comment, "\n", " ")
}
