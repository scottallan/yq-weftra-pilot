# JSONC

Decode and encode JSONC ([jsonc.org](https://jsonc.org/)): plain JSON, plus `//` line comments, `/* ... */` block comments, and a single, optional trailing comma before a closing `}` or `]`.

Use `-p jsonc` to read JSONC and `-o jsonc` to write it, exactly like the `json` format (`.jsonc` files are also auto-detected by extension). `jsonc` decodes comment-free, trailing-comma-free JSON identically to `json` - it is strictly additive and does not change `json`'s own behaviour.

Comments are attached to the node they annotate using the same head/line/foot comment mechanism yq already uses for YAML, so a comment on a node that isn't touched by an expression survives edits made elsewhere in the document - it isn't just a byte-for-byte passthrough. Where a JSON structure makes the attachment position ambiguous, yq follows the closest analogous rule already used by the YAML decoder:

- A comment on its own line immediately before an object member or array element is that member/element's **head comment** (attached to the key, for object members).
- A comment trailing on the same line as a value (before or after its comma) is that value's **line comment**.
- A comment on its own line after the last member/element, before the closing `}`/`]`, is that last child's **foot comment**.
- A comment on the same line as a container's opening `{`/`[` is treated the same as a comment on its own line right after the opening bracket: it heads the first member/element, or becomes the container's own foot comment if the container has no children at all.

On encode, `-o jsonc` never writes a trailing comma back out - trailing-comma tolerance is input-only - and comments are always re-emitted as `//` line(s), regardless of whether they were originally `//` or `/* */` style in the source.

See below for examples
