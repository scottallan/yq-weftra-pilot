# Feature Spec: JSONC (JSON with comments and optional trailing comma) support

- **Status:** Draft — spec only, no implementation yet
- **Upstream issue:** [mikefarah/yq#2536](https://github.com/mikefarah/yq/issues/2536)
- **Source of truth for scope:** the issue text quoted in the task description, plus the submitter clarification on comment preservation across mutation. Nothing else.

## 1. What is being built, and why

yq currently supports JSON as an input and output format (`-p json` / `-o json`, `Names: ["j"]`, see `pkg/yqlib/format.go`), but that format is standard JSON: it has no concept of comments, and a trailing comma before a `}` or `]` is a parse error.

Users who keep configuration in the JSONC dialect (plain JSON extended with `//` and `/* */` comments, and — in the VS Code flavor linked from the issue, https://jsonc.org/trailingcommas — an optional trailing comma before a closing `}` or `]`) currently cannot use yq to edit those files without losing the comments or hand-stripping trailing commas first. The issue's motivating example is migrating a `sing-box` JSONC config: read a `.jsonc` file, mutate a field with a yq expression, and get back JSONC that still has the original head comment and still has the mutated data — comments must not be collateral damage of an edit elsewhere in the document.

This feature adds a new, distinct `jsonc` format (input and output) that:
- Decodes standard JSON, plus `//` line comments, `/* ... */` block comments, and a single optional trailing comma before a closing `}` or `]`.
- Attaches decoded comments to the `CandidateNode` they annotate using yq's existing generic `HeadComment` / `LineComment` / `FootComment` fields (`pkg/yqlib/candidate_node.go`) — the same mechanism the YAML front end already uses — so comments on untouched nodes survive edits applied elsewhere in the tree, exactly like editing a YAML file with comments today.
- Encodes back to JSONC: valid JSON structurally, with any comments still attached to surviving nodes re-emitted in their head/line/foot positions, and never with a trailing comma.

`jsonc` is additive: it does not change the existing `json` format's behavior, error strictness, or output.

## 2. Out of scope (explicitly NOT part of this feature)

These are deliberate boundaries. If a change requires any of the following, it belongs to a different feature/issue, not this one:

1. **JSON5, not JSONC.** No unquoted/bareword object keys, no single-quoted strings, no trailing commas in contexts other than before `}`/`]`, no leading `+` on numbers, no hex/octal number literals, no multi-line strings, no `Infinity`/`NaN`. JSONC per the issue's own reference (jsonc.org) is "JSON + comments + optional trailing comma" — nothing more.
2. **Modifying the existing `json` format.** `-p json` / `-o json` keep rejecting comments and trailing commas exactly as they do today. `jsonc` is a new, separate `*Format` entry, not a lenient mode bolted onto `JSONFormat`.
3. **Multiple/repeated trailing commas.** `[1, 2,,]` or `{"a":1,,}` remain parse errors. Only a single trailing comma immediately before the closing bracket/brace is tolerated.
4. **Comments as data.** No feature reads comment text as a value, no way to set a comment via a special JSON key (the issue explicitly floats and does not request "special metadata keys" as the chosen solution — it prefers real comment tracking, per the clarification).
5. **New yq operators or CLI flags for comment manipulation.** Comments on a `jsonc`-decoded node are read/written through the operators that already exist for YAML comments (`head_comment`, `line_comment`, `foot_comment`, `... comments=...` style, etc.). No jsonc-specific comment operator is introduced.
6. **Byte-for-byte passthrough of untouched whitespace/formatting.** yq already re-serializes every format through its own encoder (indentation, spacing) rather than preserving original byte layout; jsonc follows the same house style. Preservation is of **data + comments + comment attachment**, not of exact original whitespace, blank lines, or comment column position.
7. **Comment preservation for any other format pairing** (e.g. YAML→jsonc, jsonc→YAML retaining jsonc-specific comment placement quirks beyond the normal head/line/foot model). Cross-format comment transfer works only to the extent it already works generically via `HeadComment`/`LineComment`/`FootComment` on `CandidateNode` — no jsonc-specific cross-format logic is added.
8. **Streaming/partial decode, schema validation, or JSON Pointer/patch support.** Not requested by the issue.
9. **Windows/CRLF-specific comment edge cases** beyond what the existing YAML decoder already handles for line endings.

## 3. Format registration and CLI surface

Acceptance criteria in this section are about observable CLI behavior; they intentionally do not prescribe internal type/function names.

- [ ] A new format is selectable as an **input** format via `-p jsonc` (`--input-format jsonc`) and as an **output** format via `-o jsonc` (`--output-format jsonc`), matching the exact invocation shown in the issue: `yq -P -o jsonc -p jsonc '<expr>' file.jsonc`.
- [ ] The format's formal name is `jsonc`. It appears in the `-p`/`-o` flag help text and in the "unknown format" error's list of available formats, the same way every other entry in `pkg/yqlib/format.go`'s `Formats` slice does (e.g. `please use [auto|a|yaml|y|...]` grows to include `jsonc`).
- [ ] Files with a `.jsonc` extension are auto-detected as the `jsonc` format when no explicit `-p`/`-o` is given, consistent with the existing extension-sniffing behavior (`FormatStringFromFilename` in `pkg/yqlib/format.go`), the same way `.json` already maps to `json` and `.yml`/`.yaml` map to `yaml`.
- [ ] `jsonc` is listed in `yq --help` / the formats usage doc alongside the other formats, following the existing per-format documentation convention (e.g. as `json` and other formats are documented under `pkg/yqlib/doc/usage/`).
- [ ] Generic, format-independent flags continue to work unmodified with `jsonc`: `-P` (pretty-print), `-I`/`--indent` (indent width), `-n` (null input), `-i` (in-place). None of these require jsonc-specific handling — they are already format-agnostic (`-P`, for example, works by injecting a `style=""` expression, not by calling into the encoder specially).

## 4. Decoding (`-p jsonc`) — acceptance criteria

- [ ] Any input that is valid, comment-free, trailing-comma-free JSON decodes identically (same resulting node tree, same values, same types) whether read with `-p json` or `-p jsonc`.
- [ ] A `//` comment from the start of a line to end-of-line is recognized and stripped from the value stream, outside of string literals (i.e. `"a // not a comment"` is a string containing `// not a comment`, not a comment).
- [ ] A `/* ... */` block comment (including one spanning multiple lines) is recognized and stripped from the value stream, outside of string literals.
- [ ] A comment is attached as a `HeadComment`, `LineComment`, or `FootComment` on the `CandidateNode` it annotates, using the same attachment convention already established for the YAML decoder (`pkg/yqlib/decoder_yaml.go` / the underlying YAML library's head/line/foot rules) applied to the analogous structural position in JSON — e.g., a comment on its own line(s) immediately before a key/value is that node's head comment; a comment trailing on the same line as a value is that node's line comment. Where JSON's structure makes the equivalent YAML position ambiguous or undefined, the decoder follows whatever the YAML decoder does in the closest analogous case, and this mapping is written down in the format's usage documentation (not left implicit).
- [ ] A single trailing comma immediately before a closing `}` (after the last member) or `]` (after the last element) is accepted and treated as if absent — it produces the same node tree as the same document without the trailing comma.
- [ ] A trailing comma is **only** tolerated in that one position. Any other malformed comma usage (missing comma between members, comma with nothing following but more commas, leading comma, trailing comma inside an otherwise-empty `{}`/`[]` — i.e. `{,}` or `[,]`) is a decode error, same as it would be for `-p json`.
- [ ] Decode errors for genuinely malformed input (unterminated string, invalid token, unbalanced brackets, unterminated comment) surface as errors, not as silent data loss, matching the general error-handling expectation already set by the JSON decoder.

## 5. Encoding (`-o jsonc`) — acceptance criteria

- [ ] With no comments and no trailing-comma tolerance in play, `-o jsonc` output is byte-identical to `-o json` output for the same node tree and the same indent/pretty settings (jsonc's structural encoding is JSON; it does not diverge from the `json` encoder's number/string/formatting rules).
- [ ] `-o jsonc` **never** emits a trailing comma, regardless of whether the source document had one. Trailing-comma tolerance is input-only, per the clarification.
- [ ] A node carrying a `HeadComment` is emitted with that comment as one or more `//` lines (or an equivalent representation) immediately preceding the node in the output, before its key/value is written.
- [ ] A node carrying a `LineComment` is emitted with that comment on the same output line as the node's value/closing token.
- [ ] A node carrying a `FootComment` is emitted with that comment immediately following the node, before the next sibling or the parent's closing bracket.
- [ ] Round-tripping a `jsonc` document through `-p jsonc | -o jsonc` with the identity expression (`.`) and no mutation preserves all comments, in their original head/line/foot roles, and all data — this is necessary but explicitly **not sufficient** on its own to satisfy the feature (see clarification: passthrough-only round-tripping does not by itself satisfy the request).

## 6. Comment preservation under mutation — the feature's acceptance bar

This is the actual bar the issue sets, and it is a distinct, stronger requirement than passthrough round-tripping (§5's last item).

- [ ] **The issue's worked example is a literal acceptance test.** Given the input:

  ```jsonc
  // sing-box 1.12.0 migration
  {
    "dns": {
      "servers": [
        {
          "address": "local",
        }
      ]
    }
  }
  ```

  running:

  ```bash
  yq -P -o jsonc -p jsonc '.dns.servers[0].type = .dns.servers[0].address' data1.jsonc
  ```

  produces output equivalent to:

  ```jsonc
  // sing-box 1.12.0 migration
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
  ```

  i.e. the head comment on the root document survives an edit that adds a new sibling field several levels deeper, and the trailing comma from the input is gone (not re-emitted).

- [ ] A comment attached to a node that is **not** touched by the expression survives when a **sibling** node is added, removed, or modified.
- [ ] A comment attached to a node survives when that same node's **value** is changed (e.g. `.dns.servers[0].address = "8.8.8.8"` keeps any comment that was attached to the `address` key/value).
- [ ] A comment attached to a node is **removed** (not orphaned or misattached) when that specific node is deleted via `del(...)`, consistent with how deleting a commented YAML node already behaves.
- [ ] Adding a brand-new node (no prior comment) does not fabricate a comment, and does not steal an adjacent node's comment.

## 7. Non-functional / consistency criteria

- [ ] `jsonc` is added to `pkg/yqlib/format.go`'s `Formats` list following the existing `Format{FormalName, Names, EncoderFactory, DecoderFactory}` pattern used by every other format in that file — this is stated here only as an observable consistency requirement (the format must behave like a first-class citizen of the existing format list: appears in help text, in `GetAvailableInputFormats`/`GetAvailableOutputFormats`, etc.), not as an implementation instruction.
- [ ] The feature is covered by the same kind of test/documentation-generation convention already used for other formats (e.g. `pkg/yqlib/json_test.go`'s `formatScenario`-driven tests that double as the source for generated usage docs) — i.e., acceptance is demonstrated through the project's existing test style, not a one-off script.
- [ ] `scripts/spelling.sh` / project spellcheck passes on any new documentation.
- [ ] No regression in existing `json`, `yaml`, or other format test suites.

## 8. Explicitly deferred / open questions for implementation to resolve (not blocking this spec)

These are things the implementation must make a concrete decision on, but this spec does not mandate a specific answer for, since the issue and clarification don't specify one:

- The exact short alias (if any) added to `jsonc`'s `Names` list (e.g. whether `jc` is offered) — the issue only ever invokes the formal name `jsonc`, so only `-p jsonc`/`-o jsonc` are contractually required.
- The precise head/line/foot attachment rule for comments in structurally-ambiguous positions not exercised by the issue's example (e.g. a comment on its own line directly before a closing `]`) — implementation must pick the closest YAML-decoder analog and document it, per §4.
