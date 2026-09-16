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

## Migrate a JSONC config, keeping its comments
The upstream use case ([mikefarah/yq#2536](https://github.com/mikefarah/yq/issues/2536)): a `//` head comment and a trailing comma survive an edit that adds a new sibling field several levels deep, and the trailing comma is not re-emitted.

Given a sample.jsonc file of:
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
then
```bash
yq -P -p jsonc -o jsonc '.dns.servers[0].type = .dns.servers[0].address' sample.jsonc
```
will output
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

## Comments on untouched nodes survive editing a sibling
Adding a new field does not disturb comments already attached elsewhere in the document.

Given a sample.jsonc file of:
```jsonc
{
  // comment on a
  "a": 1,
  "b": 2 // line comment on b
}

```
then
```bash
yq -P -p jsonc -o jsonc '.c = 3' sample.jsonc
```
will output
```jsonc
{
  // comment on a
  "a": 1,
  "b": 2, // line comment on b
  "c": 3
}
```

## Changing a value keeps its comment
Given a sample.jsonc file of:
```jsonc
{
  "address": "local" // comment
}

```
then
```bash
yq -P -p jsonc -o jsonc '.address = "8.8.8.8"' sample.jsonc
```
will output
```jsonc
{
  "address": "8.8.8.8" // comment
}
```

## Deleting a node removes its comment
Given a sample.jsonc file of:
```jsonc
{
  // comment on a
  "a": 1,
  "b": 2 // line comment on b
}

```
then
```bash
yq -P -p jsonc -o jsonc 'del(.a)' sample.jsonc
```
will output
```jsonc
{
  "b": 2 // line comment on b
}
```

## A new sibling does not steal an existing comment
Adding `.newThing` does not fabricate a comment for it, nor does it inherit `.address`'s comment.

Given a sample.jsonc file of:
```jsonc
{
  "address": "local" // comment
}

```
then
```bash
yq -P -p jsonc -o jsonc '.newThing = "value"' sample.jsonc
```
will output
```jsonc
{
  "address": "local", // comment
  "newThing": "value"
}
```

## A comment right after an array's opening bracket heads the first element
This mirrors how yq's YAML decoder attaches a comment that appears right after a sequence starts.

Given a sample.jsonc file of:
```jsonc
{
  "name": [
    // under-name-comment
    "first-array-child"
  ]
}

```
then
```bash
yq -P -p jsonc -o jsonc '.' sample.jsonc
```
will output
```jsonc
{
  "name": [
    // under-name-comment
    "first-array-child"
  ]
}
```

## A single trailing comma is tolerated but never re-emitted
Trailing-comma tolerance is input only - `-o jsonc` never writes one back out.

Given a sample.jsonc file of:
```jsonc
{"a": 1, "b": 2,}

```
then
```bash
yq -P -p jsonc -o jsonc '.' sample.jsonc
```
will output
```jsonc
{
  "a": 1,
  "b": 2
}
```

## /* block */ comments are re-emitted as // comments
yq does not preserve the original comment delimiter style, only its content and attachment.

Given a sample.jsonc file of:
```jsonc
{
  /* multi
     line */
  "x": 1
}

```
then
```bash
yq -P -p jsonc -o jsonc '.' sample.jsonc
```
will output
```jsonc
{
  // multi
  // line
  "x": 1
}
```

