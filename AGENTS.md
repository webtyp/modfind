# AGENTS.md — webtyp/modfind

Working notes for AI agents operating in this repository. End-user docs: [README.md](README.md).

## What this repo is

The single place in the webtyp ecosystem that answers **"where does a Go module live on disk"**:
`go list -m -json all` discovery (cached per project root), writable/read-only classification, the
developer's own checkout of a module inside the workspace (`*.code-workspace` root). How a path is
*printed* (`./…`, `~/…`) is not this repo's concern: that is `webtyp.com/filepath` (`Short`,
`Tilde`).

## This repo does NOT compile to WASM. The standard library is legitimate here.

It is tool-side code (`webtyp/app`, `webtyp/devflow`, ssr, image tooling).
`os`, `os/exec`, `path/filepath`, `encoding/json`, `bytes`, `strings` are correct here — do **not**
replace them with `webtyp.com/*` browser packages. Errors still go through `webtyp.com/fmt`
(`fmt.Err(...)`) like the existing code.

## The build that defines "done"

```bash
go install webtyp.com/devflow/cmd/gotest@latest   # once
gotest
```

## Rules

- Tests live in `tests/` (`package modfind_test`, public API only). A root-level test is allowed
  only with a top-of-file comment justifying the unexported identifier it needs —
  `discover_test.go` is one (it replaces the private go-list runner). **Never export a symbol so a
  test can reach it.**
- No toolchain in unit tests; filesystem tests build their tree under `t.TempDir()`; the log sink is
  captured with `SetLog`.
- No test reads or writes the developer's real home: tests point `$HOME` at a temp dir with
  `t.Setenv("HOME", …)`.
- Every repeated string (marker file extensions, skipped directory names) is a named constant.
- Reuse before writing: this repo is the owner of module/workspace discovery. If another webtyp repo
  walks `go.mod` files or climbs to a `*.code-workspace`, the fix is to make it call this repo.
