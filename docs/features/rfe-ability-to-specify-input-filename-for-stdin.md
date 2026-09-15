# RFE: ability to specify input filename for stdin

## Source

Upstream issue: [mikefarah/yq#1876](https://github.com/mikefarah/yq/issues/1876) — "RFE: ability to specify input filename for stdin".

## Background

Since [mikefarah/yq#1484](https://github.com/mikefarah/yq/issues/1484), yq auto-detects the input (and, by default, output) format from a real input file's extension (e.g. `file.json` is treated as JSON). Today, this detection is only available when yq is given a filename argument. When input arrives via stdin, yq has no filename to inspect, so it falls back to the `yaml` default (see `pkg/yqlib/format.go:FormatStringFromFilename`, `cmd/utils.go:configureInputFormat`).

Tools that pipe content to yq through stdin (e.g. editor/formatter integrations) often know the "logical" filename of the content even though no such file exists on disk yet, or the file isn't saved. They want yq's extension-based format detection to work in that case too, without needing to write the content to a real file first.

## What is being built

A new flag, `--stdin-filename <filename>`, that lets the caller tell yq what filename to use *for format-detection purposes only* when reading from stdin. The filename does not need to refer to a real file, and yq must not attempt to open, stat, or otherwise access it on disk — it is used purely as a string whose extension feeds the same logic that already infers format from a real input file's extension.

This mirrors how other formatters expose the same capability (e.g. dprint's `--stdin <filename>`, ruff format's `--stdin-filename <filename>`), and the issue explicitly requests the latter naming.

## Non-goals addressed by clarification

When both an explicit input format (`--input-format` / `-p`, given any value other than `auto`/`a`) and `--stdin-filename` are supplied, the explicit input format wins. `--stdin-filename` only feeds the *automatic* detection path — the same path a real input file's extension feeds today — and never overrides a format the user stated explicitly. This is consistent with the issue's own framing of the flag as "just an indication of the filename for format detection purposes."

## Acceptance Criteria

1. **Flag exists and is documented.**
   - A new persistent flag `--stdin-filename` (string, default empty) is registered alongside the other format-related flags (near `--input-format`/`-p` in `cmd/root.go`).
   - Running `yq --help` (or `yq -h`) lists `--stdin-filename` with a description that states it sets the filename used for auto format detection when reading from stdin, and that the file need not exist.
   - The flag is documented in the project's usage docs (`pkg/yqlib/doc/usage/`) following the existing convention of showing an input/output example pair (mirroring how `-p`/format auto-detection is documented today), and/or referenced from the root command's long description/example block in `cmd/root.go` where the existing auto-detection behaviour is already explained.

2. **`--stdin-filename` drives auto format detection for stdin, exactly like a real filename does for files.**
   - Given `--stdin-filename foo.json` (or `.xml`, `.toml`, `.csv`, `.tsv`, `.ini`, `.properties`, any other extension yq already recognises via `FormatStringFromFilename`) and no explicit `--input-format`, piping content on stdin (i.e. no file argument, or `-` as the file argument) causes yq to parse stdin using the detected format — identical to running `yq . realfile.json` on stdin content of the same shape, and identical to the format yq would select if the same filename were passed as a real, existing file.
   - When `--output-format`/`-o` is likewise left on `auto`, the output format for that invocation is also driven by the detected format, exactly as it is today for real input files (per the existing `configureInputFormat`/`isAutomaticOutputFormat` behaviour) — i.e. `--stdin-filename` produces the same input/output format pairing as an equivalent real file would.
   - An extension yq does not recognise (or no extension) causes the same fallback to `yaml` that an unrecognised/absent extension on a real filename causes today. No error is raised solely because the extension is unrecognised.

3. **The filename is never resolved against the filesystem.**
   - Supplying `--stdin-filename` with a path to a file that does not exist (in any directory, including nonexistent directories) succeeds and does not raise a "file not found" or similar I/O error. yq must not `open`/`stat` the `--stdin-filename` value.
   - Supplying `--stdin-filename` with a path to a file that *does* exist, but is different from what's on stdin, still reads the actual piped content from stdin (never from the named file) — the flag affects format selection only, never the source of the data.

4. **Explicit `--input-format`/`-p` takes precedence over `--stdin-filename`.**
   - When both `--stdin-filename <name-with-extension-X>` and `--input-format <Y>` (`Y` not `auto`/`a`) are given, stdin is parsed using format `Y`. The extension implied by `--stdin-filename` is ignored for input-format selection.
   - This precedence matches the precedence a real input filename already has today relative to an explicit `-p`: an explicit `-p` always overrides extension-based detection, whether the extension comes from a real file argument or from `--stdin-filename`.

5. **No effect when not reading from stdin.**
   - When yq is invoked with one or more real file arguments (i.e. not reading from stdin), `--stdin-filename` has no effect on format detection for those files. Existing behaviour for file arguments is unchanged.
   - When yq is invoked with `--null-input`/`-n` (no data is read at all), `--stdin-filename` has no effect and does not cause an error by merely being set.

6. **No effect on filenames reported elsewhere.**
   - `--stdin-filename` only affects format auto-detection. It must not change what yq reports as the source filename in error messages, `fileIndex`/`filename` expression variables, front-matter handling, or write-in-place behaviour — those all continue to reflect that the data came from stdin (e.g. `-`), since write-in-place and similar file-identity features are already documented as inapplicable to stdin.

7. **Tests.**
   - Following the repository's existing test conventions for format-detection flags (see `cmd/utils_test.go` table-driven tests around `inputFormat`/`configureInputFormat`, and shell-based coverage in `acceptance_tests/inputs-format.sh` / `inputs-format-auto.sh`), the feature has:
     - Unit test coverage exercising `--stdin-filename` combined with: a recognised extension, an unrecognised/absent extension, and simultaneous use with an explicit `--input-format`.
     - Acceptance-test (shell) coverage piping content through stdin with `--stdin-filename` set, asserting on the resulting output format/content.

## Explicitly Out of Scope

- **No new format-detection heuristics.** This feature does not add any new way of inferring format (e.g. content sniffing, MIME detection). It only extends the *existing* filename-extension-based detection (`FormatStringFromFilename`) to be usable when the input is stdin.
- **No filesystem access of any kind for the named file.** yq must not create, read, write, watch, or validate the existence of the path passed to `--stdin-filename`. Validating that the string is a "sensible" path/filename is not required.
- **No change to output-to-stdout naming.** This is strictly an *input* concern (format detection for data read from stdin). It does not add any equivalent `--stdout-filename`, does not affect write-in-place (`-i`), split-file output naming, or the `-` convention for output.
- **No change to precedence/behaviour for real file arguments.** Behaviour when yq is given an actual file path (existing or not) is unchanged; `--stdin-filename` is only consulted when there is no file argument (or the file argument is `-`).
- **No change to default formats.** The existing "auto-detect defaults to yaml when detection fails or is unavailable" behaviour is preserved; this feature does not change what happens when `--stdin-filename` is absent.
- **No shell/editor integration tooling.** Building or documenting integrations with specific editors or formatters (the motivating examples in the issue) is out of scope — only the yq-side flag is in scope.
