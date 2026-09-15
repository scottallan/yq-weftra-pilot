//go:build !yq_nojson

package yqlib

import (
	"bufio"
	"fmt"
	"testing"

	"github.com/mikefarah/yq/v4/test"
)

const jsoncSingBoxExample = `// sing-box 1.12.0 migration
{
  "dns": {
    "servers": [
      {
        "address": "local",
      }
    ]
  }
}
`

const jsoncSingBoxExpected = `// sing-box 1.12.0 migration
{
  "dns": {
    "servers": [
      {
        "address": "local",
        "type": "local"
      }
    ]
  }
}
`

const jsoncCommentedDoc = `{
  // comment on a
  "a": 1,
  "b": 2 // line comment on b
}
`

var jsoncScenarios = []formatScenario{
	{
		description:    "Migrate a JSONC config, keeping its comments",
		subdescription: "The upstream use case ([mikefarah/yq#2536](https://github.com/mikefarah/yq/issues/2536)): a `//` head comment and a trailing comma survive an edit that adds a new sibling field several levels deep, and the trailing comma is not re-emitted.",
		input:          jsoncSingBoxExample,
		expression:     `.dns.servers[0].type = .dns.servers[0].address`,
		expected:       jsoncSingBoxExpected,
		scenarioType:   "roundtrip",
	},
	{
		description:    "Comments on untouched nodes survive editing a sibling",
		subdescription: "Adding a new field does not disturb comments already attached elsewhere in the document.",
		input:          jsoncCommentedDoc,
		expression:     `.c = 3`,
		expected:       "{\n  // comment on a\n  \"a\": 1,\n  \"b\": 2, // line comment on b\n  \"c\": 3\n}\n",
		scenarioType:   "roundtrip",
	},
	{
		description:  "Changing a value keeps its comment",
		input:        "{\n  \"address\": \"local\" // comment\n}\n",
		expression:   `.address = "8.8.8.8"`,
		expected:     "{\n  \"address\": \"8.8.8.8\" // comment\n}\n",
		scenarioType: "roundtrip",
	},
	{
		description:  "Deleting a node removes its comment",
		input:        jsoncCommentedDoc,
		expression:   `del(.a)`,
		expected:     "{\n  \"b\": 2 // line comment on b\n}\n",
		scenarioType: "roundtrip",
	},
	{
		description:    "A new sibling does not steal an existing comment",
		subdescription: "Adding `.newThing` does not fabricate a comment for it, nor does it inherit `.address`'s comment.",
		input:          "{\n  \"address\": \"local\" // comment\n}\n",
		expression:     `.newThing = "value"`,
		expected:       "{\n  \"address\": \"local\", // comment\n  \"newThing\": \"value\"\n}\n",
		scenarioType:   "roundtrip",
	},
	{
		skipDoc:      true,
		description:  "Round trip: untouched document keeps its comments",
		input:        jsoncCommentedDoc,
		expected:     jsoncCommentedDoc,
		scenarioType: "roundtrip",
	},
	{
		description:    "A comment right after an array's opening bracket heads the first element",
		subdescription: "This mirrors how yq's YAML decoder attaches a comment that appears right after a sequence starts.",
		input:          "{\n  \"name\": [\n    // under-name-comment\n    \"first-array-child\"\n  ]\n}\n",
		expected:       "{\n  \"name\": [\n    // under-name-comment\n    \"first-array-child\"\n  ]\n}\n",
		scenarioType:   "roundtrip",
	},
	{
		description:    "A single trailing comma is tolerated but never re-emitted",
		subdescription: "Trailing-comma tolerance is input only - `-o jsonc` never writes one back out.",
		input:          "{\"a\": 1, \"b\": 2,}\n",
		expected:       "{\n  \"a\": 1,\n  \"b\": 2\n}\n",
		scenarioType:   "roundtrip",
	},
	{
		skipDoc:      true,
		description:  "A single trailing comma is tolerated in arrays too",
		input:        "[1, 2, 3,]\n",
		expected:     "[\n  1,\n  2,\n  3\n]\n",
		scenarioType: "roundtrip",
	},
	{
		description:    "/* block */ comments are re-emitted as // comments",
		subdescription: "yq does not preserve the original comment delimiter style, only its content and attachment.",
		input:          "{\n  /* multi\n     line */\n  \"x\": 1\n}\n",
		expected:       "{\n  // multi\n  // line\n  \"x\": 1\n}\n",
		scenarioType:   "roundtrip",
	},
	{
		skipDoc:      true,
		description:  "A comment on its own line before a closing } is the last member's foot comment",
		input:        "{\n  \"a\": 1\n  // foot\n}\n",
		expected:     "{\n  \"a\": 1\n  // foot\n}\n",
		scenarioType: "roundtrip",
	},
	{
		skipDoc:      true,
		description:  "A comment on its own line before a closing ] is the last element's foot comment",
		input:        "[\n  1,\n  2\n  // foot\n]\n",
		expected:     "[\n  1,\n  2\n  // foot\n]\n",
		scenarioType: "roundtrip",
	},
	{
		skipDoc:      true,
		description:  "A head comment on a scalar-only document survives",
		input:        "// just a string\n\"hello\"\n",
		expected:     "// just a string\n\"hello\"\n",
		scenarioType: "roundtrip",
	},
	{
		skipDoc:      true,
		description:  "Empty object",
		input:        "{}",
		expected:     "{}\n",
		scenarioType: "roundtrip",
	},
	{
		skipDoc:      true,
		description:  "Empty array",
		input:        "[]",
		expected:     "[]\n",
		scenarioType: "roundtrip",
	},
	{
		skipDoc:       true,
		description:   "Decode error: a second trailing comma is not tolerated",
		input:         "[1,2,,]",
		expectedError: `bad file 'sample.yml': line 1: unexpected ",", expected a value`,
		scenarioType:  "decode-error",
	},
	{
		skipDoc:       true,
		description:   "Decode error: a leading comma in an object is not tolerated",
		input:         "{,}",
		expectedError: `bad file 'sample.yml': line 1: expected a string key or '}', got ","`,
		scenarioType:  "decode-error",
	},
	{
		skipDoc:       true,
		description:   "Decode error: a leading comma in an array is not tolerated",
		input:         "[,]",
		expectedError: `bad file 'sample.yml': line 1: unexpected ",", expected a value`,
		scenarioType:  "decode-error",
	},
	{
		skipDoc:       true,
		description:   "Decode error: missing comma between members",
		input:         "{\"a\":1 \"b\":2}",
		expectedError: `bad file 'sample.yml': line 1: expected ',' or '}', got "\"b\""`,
		scenarioType:  "decode-error",
	},
	{
		skipDoc:       true,
		description:   "Decode error: unterminated string literal",
		input:         `{"a": "unterminated`,
		expectedError: `bad file 'sample.yml': line 1: unterminated string literal`,
		scenarioType:  "decode-error",
	},
	{
		skipDoc:       true,
		description:   "Decode error: unterminated block comment",
		input:         `{"a": 1 /* unterminated`,
		expectedError: `bad file 'sample.yml': line 1: unterminated block comment`,
		scenarioType:  "decode-error",
	},
	{
		skipDoc:       true,
		description:   "Decode error: invalid literal",
		input:         `{"a": tru}`,
		expectedError: `bad file 'sample.yml': line 1: invalid value "tru": json: invalid character as true`,
		scenarioType:  "decode-error",
	},
}

func testJsoncScenario(t *testing.T, s formatScenario) {
	prefs := ConfiguredJSONPreferences.Copy()
	prefs.Indent = 2
	prefs.UnwrapScalar = false
	prefs.ColorsEnabled = false

	switch s.scenarioType {
	case "", "roundtrip":
		test.AssertResultWithContext(t, s.expected, mustProcessFormatScenario(s, NewJSONCDecoder(), NewJSONCEncoder(prefs)), s.description)
	case "decode-error":
		result, err := processFormatScenario(s, NewJSONCDecoder(), NewJSONCEncoder(prefs))
		if err == nil {
			t.Errorf("Expected error '%v' but it worked: %v", s.expectedError, result)
		} else {
			test.AssertResultComplexWithContext(t, s.expectedError, err.Error(), s.description)
		}
	default:
		panic(fmt.Sprintf("unhandled scenario type %q", s.scenarioType))
	}
}

func documentJsoncScenario(_ *testing.T, w *bufio.Writer, i interface{}) {
	s := i.(formatScenario)
	if s.skipDoc {
		return
	}

	writeOrPanic(w, fmt.Sprintf("## %v\n", s.description))
	if s.subdescription != "" {
		writeOrPanic(w, s.subdescription)
		writeOrPanic(w, "\n\n")
	}

	writeOrPanic(w, "Given a sample.jsonc file of:\n")
	writeOrPanic(w, fmt.Sprintf("```jsonc\n%v\n```\n", s.input))

	writeOrPanic(w, "then\n")
	expression := s.expression
	if expression == "" {
		expression = "."
	}
	writeOrPanic(w, fmt.Sprintf("```bash\nyq -P -p jsonc -o jsonc '%v' sample.jsonc\n```\n", expression))
	writeOrPanic(w, "will output\n")

	prefs := ConfiguredJSONPreferences.Copy()
	prefs.Indent = 2
	prefs.UnwrapScalar = false
	prefs.ColorsEnabled = false
	writeOrPanic(w, fmt.Sprintf("```jsonc\n%v```\n\n", mustProcessFormatScenario(s, NewJSONCDecoder(), NewJSONCEncoder(prefs))))
}

func TestJsoncScenarios(t *testing.T) {
	for _, tt := range jsoncScenarios {
		testJsoncScenario(t, tt)
	}
	genericScenarios := make([]interface{}, len(jsoncScenarios))
	for i, s := range jsoncScenarios {
		genericScenarios[i] = s
	}
	documentScenarios(t, "usage", "jsonc", genericScenarios, documentJsoncScenario)
}

// TestJsoncDecodeMatchesJsonForPlainInput checks the §4 acceptance bar: any
// input that is valid, comment-free, trailing-comma-free JSON must decode to
// the same node tree - and so encode to the same bytes - whether read as
// `json` or `jsonc`.
func TestJsoncDecodeMatchesJsonForPlainInput(t *testing.T) {
	inputs := []string{
		`{"a":50.0,"b":[1,2.5,"x"],"c":null,"d":true,"e":-7,"f":1.5e-3}`,
		`[]`,
		`{}`,
		`[1,2,3]`,
		`"just a string"`,
		`42`,
		`null`,
		`true`,
		`false`,
		`{"nested":{"deeply":{"array":[1,[2,3],{"k":"v"}]}}}`,
		`[{},{},[]]`,
		`{"unicode":"café"}`,
		`9223372036854775807`,
	}
	for _, in := range inputs {
		prefs := ConfiguredJSONPreferences.Copy()
		prefs.UnwrapScalar = false
		jsonOut := mustProcessFormatScenario(formatScenario{input: in}, NewJSONDecoder(), NewJSONEncoder(prefs))
		jsoncOut := mustProcessFormatScenario(formatScenario{input: in}, NewJSONCDecoder(), NewJSONCEncoder(prefs))
		test.AssertResultWithContext(t, jsonOut, jsoncOut, in)
	}
}
