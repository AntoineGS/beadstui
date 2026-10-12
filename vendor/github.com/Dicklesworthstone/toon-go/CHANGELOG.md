# Changelog

All notable changes to [toon-go](https://github.com/Dicklesworthstone/toon-go) are documented here.

This project has no tagged releases. All entries below track commits on the `main` branch.

> **toon-go** provides Go bindings for [TOON](https://github.com/Dicklesworthstone/toon_rust) (Token-Optimized Object Notation), wrapping the `tru` CLI binary via subprocess. Requires Go 1.21+.

---

## Unreleased

### Binary Discovery

#### Accept `toon` as an alternate binary name (2026-03-02)

[`d140945`](https://github.com/Dicklesworthstone/toon-go/commit/d14094544b725fc630ff11080b836f464667c2bc) -- Closes [#1](https://github.com/Dicklesworthstone/toon-go/issues/1)

The `toon_rust` project began shipping its release binary as `toon` instead of `tru`. The previous code hard-banned any binary named `toon` or `toon.exe` (to avoid collisions with a Node.js wrapper), which caused `Available()` to return `false` even when the correct Rust binary was present.

- `findTruBinary` now searches PATH for both `tru` and `toon`, in that order.
- Removed the filename-based hard ban from `isToonRustBinary`. Detection now relies exclusively on `--help` / `--version` output fingerprinting, which is sufficient to distinguish the Rust implementation from any same-named wrapper.

#### Robust tru resolution with identity verification (2026-01-24)

[`cec6e36`](https://github.com/Dicklesworthstone/toon-go/commit/cec6e36c2d9794e982825fd0e4ad7ebc7d7a35f6)

The original binary lookup did simple `os.Stat` checks -- it would silently use any unrelated binary that happened to be named `tru`.

- `TOON_TRU_BIN` and `TOON_BIN` environment variables now accept both absolute paths and bare command names (resolved via `PATH`).
- Introduced `resolveTruCandidate` helper that distinguishes path-like values from command names.
- Introduced `isToonRustBinary`, which probes `--help` for "reference implementation in rust" and `--version` for a `tru ` or `toon_rust ` prefix. Every candidate (env vars, PATH, common paths) must pass this check.

### Format Detection

#### Simplify DetectFormat fallback logic (2026-01-26)

[`3a62b15`](https://github.com/Dicklesworthstone/toon-go/commit/3a62b152d28ef50a917b5f6d3f68b8e60e12ebd1)

- `DetectFormat` no longer attempts TOON-specific heuristics (checking for `": "` or `"]:"` patterns). Any input that is not valid JSON now defaults to `FormatTOON`; invalid TOON will fail gracefully at decode time.
- Eliminates a class of false-negative misdetections where valid TOON input lacked the expected key patterns.

#### Fix DetectFormat for JSON scalars (2026-01-24)

[`250e80b`](https://github.com/Dicklesworthstone/toon-go/commit/250e80b202fb0778e88629b21ebde0207e562379)

- `DetectFormat` previously only recognized JSON beginning with `{` or `[`. It now uses `json.Unmarshal` as the primary check, correctly detecting scalar JSON values (`"hello"`, `123`, `true`, `null`).
- Added test cases for JSON scalar detection.

### Licensing

#### Update to MIT with OpenAI/Anthropic Rider (2026-02-21)

[`81d4f28`](https://github.com/Dicklesworthstone/toon-go/commit/81d4f281545a8413f312d5c3390d874c19d4a387) | [`791a9cf`](https://github.com/Dicklesworthstone/toon-go/commit/791a9cf75f21aeb7007b0a7e6e331eeac1b68210)

- LICENSE replaced from plain MIT to "MIT License (with OpenAI/Anthropic Rider)". The rider excludes OpenAI, Anthropic, and their affiliates from all granted rights unless Jeffrey Emanuel provides express written permission.
- README license section updated to match.

#### Add initial MIT LICENSE (2026-01-24)

[`e044b09`](https://github.com/Dicklesworthstone/toon-go/commit/e044b09590e8527bb1992cfbad6600745cf22ce7)

- Added MIT LICENSE file to the repository.

### Repository Metadata

#### GitHub social preview image (2026-02-21)

[`30b6728`](https://github.com/Dicklesworthstone/toon-go/commit/30b672863f5dd8c9813cf7d1373fdc05992bfc56)

- Added `gh_og_share_image.png` (1280x640) for Open Graph social card when sharing the repository URL.

### Initial Library (2026-01-24)

[`10236ac`](https://github.com/Dicklesworthstone/toon-go/commit/10236ac156a3a5f2d9bb5534dfeceaa8cb077d10)

First commit. Full Go binding library for TOON via the `tru` CLI subprocess.

#### Core API

| Function | Purpose |
|---|---|
| `Encode(data any) (string, error)` | Convert a Go value to TOON using default options |
| `EncodeWithOptions(data any, opts EncodeOptions) (string, error)` | Encode with custom key-folding, delimiter, and indent |
| `Decode(toonStr string, v any) error` | Parse TOON into a Go value using default options |
| `DecodeWithOptions(toonStr string, opts DecodeOptions, v any) error` | Decode with expand-paths and strict-mode control |
| `DecodeToJSON(toonStr string) (string, error)` | Parse TOON, return the raw JSON string |
| `DecodeToJSONWithOptions(toonStr string, opts DecodeOptions) (string, error)` | Same, with options |
| `DecodeToValue(toonStr string) (any, error)` | Parse TOON into `any` |
| `DecodeToValueWithOptions(toonStr string, opts DecodeOptions) (any, error)` | Same, with options |
| `DetectFormat(input string) Format` | Returns `FormatJSON`, `FormatTOON`, or `FormatUnknown` |
| `Convert(input string) (string, Format, error)` | Auto-detect format, convert to the other |
| `Available() bool` | Check whether the `tru` binary is reachable |
| `TruPath() (string, error)` | Return resolved path to `tru` |

#### Options

- **`EncodeOptions`** -- `KeyFolding` (off / safe), `FlattenDepth`, `Delimiter` (`,` / `\t` / `|`), `Indent`
- **`DecodeOptions`** -- `ExpandPaths`, `Strict`

#### Error Handling

All errors are wrapped in `*ToonError` carrying `Code`, `Message`, and `Cause`.

| Code | Constant | Meaning |
|------|----------|---------|
| 10 | `ErrCodeEncodeFailed` | Encoding failed |
| 11 | `ErrCodeDecodeFailed` | Decoding failed |
| 13 | `ErrCodeTruNotFound` | `tru` binary not available |

#### Binary Resolution Order

1. `TOON_TRU_BIN` environment variable
2. `TOON_BIN` environment variable
3. `tru` then `toon` in `PATH`
4. Common paths: `/usr/local/bin/tru`, `/usr/bin/tru`, `/data/tmp/cargo-target/{release,debug}/tru`

#### Test Suite

20 tests covering encode, decode, roundtrip, format detection, error paths, and options pass-through, plus two benchmarks (`BenchmarkEncode`, `BenchmarkDecode`).

#### Files Introduced

| File | Lines | Role |
|------|-------|------|
| `toon.go` | 371 | Library implementation |
| `toon_test.go` | 502 | Test suite |
| `go.mod` | 3 | Module declaration (Go 1.21) |
| `README.md` | 218 | API reference, quick start, integration patterns |
