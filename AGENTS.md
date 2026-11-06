## Project Overview

A batteries-included Go backend framework.

## Coding Style & Patterns

- Follow existing code patterns and keep changes focused.
- Format Go code with `gofmt`.
- Use `any` instead of `interface{}`.
- Give every goroutine a clear owner and shutdown mechanism. Propagate
  `context.Context` for cancellable work and set timeouts where operations
  could otherwise wait indefinitely.
- Prefer sentinel errors declared with `var ErrX = errors.New("...")` when
  callers need to distinguish failures; check them with `errors.Is`.
- Use struct error types only when callers need additional error data or
  behavior; inspect them with `errors.As`.
- Wrap external errors with useful context using `fmt.Errorf("context: %w", err)`
  when preserving the underlying error helps callers.

## Testing

- Add a regression test for bug fixes.
- Test new behavior and meaningful edge cases.
- Reuse existing test patterns and helpers.
- Keep tests deterministic; avoid sleeps for synchronization.
- Run `make test` after code changes.
- Run `go test -race ./...` when changing concurrency.

## Verification & Build Commands

- Format code: `make fmt`
- Run unit tests: `make test`
- Lint code: `make lint`
- After dependency changes: `go mod tidy`
- Run checks appropriate to the change.

<!-- CODEGRAPH_START -->

## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow. Name a file or symbol in the query to read its current line-numbered source. If it's listed but deferred, load it by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` prints the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->
