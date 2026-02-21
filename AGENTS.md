# AGENTS.md

Guidance for AI/code agents working in this imgproxy fork.

## Scope and Branching

- Repository: `851-labs/imgproxy`
- This fork's active work is based on `upstream/version/4` (not `upstream/master`).
- For feature work in this fork, compare and PR against `version/4` to avoid huge unrelated diffs.
- Current custom fork features include:
  - `sticker_trace` (`st`)
  - `blurhash_image` (`bhi`)

## Quick Start Commands

- Build: `make build`
- Run local binary: `make run -- <args>`
- Build and run: `make build-and-run -- <args>`
- Format Go: `make fmt`
- Lint Go: `make lint-go`
- Test (containerized): `make test`

Notes:

- The Makefile runs many tasks inside `ghcr.io/imgproxy/imgproxy-base:v4-dev`.
- Native `go test` on macOS often fails if local `libvips/pkg-config` setup is incomplete.
- If image fixture tests fail, initialize submodule:
  - `git submodule update --init --recursive`

## High-Level Architecture

- App composition/root: `imgproxy.go`
- CLI entrypoint: `cli/main.go`
- Router and middleware: `server/router.go`, `server/middlewares.go`
- Main handler: `handlers/processing/handler.go`
- URL/signature split: `handlers/path.go`
- Option parsing:
  - parser config: `options/parser/config.go`
  - route option dispatch: `options/parser/processing_options.go`
  - parsing and validation: `options/parser/apply.go`, `options/parser/parse.go`
  - URL decode: `options/parser/url.go`
  - keys: `options/keys/keys.go`
- Processing pipeline:
  - order: `processing/processing.go`
  - pipeline executor/context: `processing/pipeline.go`
  - typed processing getters: `processing/options.go`
- Response headers/caching: `server/responsewriter/writer.go`
- Security checks: `security/signature.go`, `security/*`

## Request Lifecycle (Mental Model)

1. Route matches `GET /*` in `imgproxy.go`.
2. Middleware stack applies (monitoring/error/panic/CORS/secret as configured).
3. `handlers/processing` parses `{signature}/{path}` and verifies signature/source.
4. `options/parser` builds `*options.Options` and source URL.
5. If `raw=true`, request is delegated to stream handler.
6. Otherwise image is fetched (`imagedata/factory`) and processed (`processing/processing.go`).
7. Result is saved and emitted with cache/canonical/CSP headers via response writer.

## Code Patterns To Follow

### Config pattern (repo-wide)

Most packages use:

- `type Config struct { ... }`
- `NewDefaultConfig() Config`
- `LoadConfigFromEnv(*Config) (*Config, error)`
- `Validate() error`

Follow this structure for new configurable modules.

### Option parser pattern

When adding a new URL processing option:

1. Add keys in `options/keys/keys.go`
2. Register names/aliases in `options/parser/processing_options.go`
3. Parse and validate in `options/parser/apply.go`
4. Add typed accessors in `processing/options.go`
5. Add parser tests in `options/parser/processing_options_test.go`

Important:

- `AllowedProcessingOptions` checks exact URL token before normalization; include aliases if users send aliases.
- Invalid bool values do not hard-fail; parser logs warning and treats as `false`.

### Processing step pattern

For a new processing transform:

1. Create `processing/<feature>.go` with `func (p *Processor) feature(c *Context) error`
2. Early-return when disabled.
3. Decide explicit animated behavior (process, skip, or special path).
4. If geometry changes, call `c.CalcParams()` so downstream steps use updated dimensions.
5. Use `CopyMemory()` where required by libvips/random-access operations.
6. Add cancellation checks for long loops (`ctx.Err()` or timeout checks).
7. Insert step in `processing/processing.go` in intentional order.
8. Add tests in `processing/<feature>_test.go`.

### Testing pattern

- Uses `testify/suite` plus `testutil.LazySuite` helpers.
- Parser tests are typically `TestParsePath...`.
- Processing tests often rely on `testdata` fixtures + hash matchers (`testutil/image_hash_cache_matcher.go`).

## Custom Option Implementation Checklist

For any new processing flag, complete all of:

- [ ] `options/keys/keys.go`
- [ ] `options/parser/processing_options.go`
- [ ] `options/parser/apply.go`
- [ ] `processing/options.go`
- [ ] `processing/<new_step>.go`
- [ ] pipeline insertion in `processing/processing.go`
- [ ] parser tests
- [ ] processing tests
- [ ] `go.mod`/`go.sum` update if new dependency is added

## Repo Gotchas

- `make lint-clang` target is effectively broken due to a Makefile typo (`ling-clang`) plus fallback no-op rule.
- Full `./processing` tests can fail if `testdata/test-images` submodule is empty.
- `options.Get[T]` panics on type mismatch; never reuse keys with different value types.
- `raw=true` bypasses processing pipeline entirely.
- Security options in URL are denied unless explicitly enabled (`AllowSecurityOptions`).

## PR and Validation Guidance

Before PR:

- Run `make fmt`
- Run `make lint-go`
- Run `make test` (or at least affected package tests in base container)

PR target:

- Use `version/4` as base for this fork's feature line.
- Avoid targeting `master` for `version/4`-line changes.
