---
PLAN: "feat: local checkouts and workspace root"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — modfind: where the developer edits a module

Phase **A2 (gate)** of the master plan
`SOURCE_SELECTION_MASTER_PLAN.md` (orchestration only — everything this plan needs is inline).
`webtyp/devflow` and `webtyp/app` wait for the tag this plan produces.

Read [AGENTS.md](../AGENTS.md) first. Critical rules, repeated here:

- This repo is **backend tooling**: the standard library (`os`, `path/filepath`, `strings`,
  `encoding/json`) is legitimate. Do NOT replace it with `webtyp.com/*` browser packages.
- No test reads or writes the developer's real home; no unit test runs the Go toolchain.
- Tests live in `tests/` (`package modfind_test`, public API only). A root-level test needs a
  top-of-file justification. **Never export a symbol so a test can reach it.**
- Every repeated string is a named constant.

## Why

`Finder.Discover` returns, for a dependency, its directory in the **module cache**
(`/home/<user>/go/pkg/mod/webtyp.com/components@v0.8.7`). That copy is read-only and is not where
the developer edits. In this workspace almost every dependency also exists as a normal checkout
(`~/Dev/Project/webtyp/components`), found by climbing to the outermost directory holding a
`*.code-workspace` file and walking the `go.mod` files under it. That logic exists today, private,
in `webtyp/devflow` (`WorkspaceRoot`, `findAllModules`). It moves here, so every tool resolves
module paths the same way. (Printing a path as `~/...` is NOT this repo's concern: it goes to
`webtyp.com/fmt` next to `PathShort`, in a sibling plan.)

## Design gate

**1. Prior art.**
- `go env GOWORK` / `go.work` (Go): a workspace file lists local module dirs explicitly. Strong, but
  the developer must keep it in sync by hand, and these projects do not use one.
- `gopls` (Go): resolves a symbol to the module cache unless a `replace`/`go.work` points elsewhere,
  which is the behaviour we are fixing.
- VS Code multi-root workspaces: the `*.code-workspace` file marks the root of everything the
  developer has checked out, which is the marker this ecosystem already adopted in `devflow`.
- Rust `cargo`'s `[patch]` and Node's `npm link`: explicit local overrides, both opt-in per dependency.

This ecosystem differs because the workspace marker already exists and every repo is checked out
under it. Discovering the checkout by module path needs **zero** per-project configuration, and the
walk measures ~30 ms for 169 modules.

**2. Novice-name test.**
- `Module.LocalDir`: "the module's local directory" — the developer's own checkout.
- `Module.SourceDir()`: "the directory to read this module's source from".
- `WorkspaceRoot(dir)`: unchanged name, moved verbatim from devflow, so its consumers keep the same word.
- `WorkspaceModules(root)`: "the modules in the workspace".
- `Discoverer`: "something that discovers the modules of a project". It is the `-er` interface Go
  uses for one-method contracts (`io.Reader`, `fmt.Stringer`).

**3. Complexity ledger.**
```
Concepts the developer must learn   +3 (LocalDir/SourceDir, WorkspaceModules, Discoverer) / −2 in phase G (Seed, Dirs);
                                    WorkspaceRoot only moves; WorkspaceRootFrom and the marker constant become private
Files they must touch to do X       +0 / −0
Lines at the call site              −N in devflow (two walks deleted)
Ways to do the same thing           +0 / −2  (devflow's go.mod walk dies in phase B; Dirs ≡ Discover + m.Dir dies in phase G)
```

**4. Where it belongs.** modfind's single concern is "where a Go module lives on disk". A module's
local checkout is the same question. Devflow keeps no copy.

**5. What it deletes.** In later phases: `devflow/workspace.go` (`WorkspaceRoot`,
`WorkspaceRootFrom`, `WorkspaceFileExt`, `skipWalkDir`), `devflow`'s `findAllModules` walk and
its `FindDependentModules` walk. This plan only adds the replacements.

## Stage 0 — test layout

Repo rule: tests live in `tests/` (`package modfind_test`, public API only). A test stays at the root
only with a top-of-file comment justifying the unexported identifier it needs. **Never export a
symbol so a test can reach it.**

- `discover_test.go` STAYS at the root (`package modfind`). Add this comment as its first lines:
  ```go
  // Root-level test (justified): Discover must be tested without the Go toolchain, which requires
  // replacing the unexported go-list runner (f.run). Exporting a runner only for tests is forbidden
  // by the ecosystem rule, and no consumer needs another runner.
  ```
  The `LocalDir` tests of Stage 3 go in this same file, for the same reason.
- Tests that only need exported API (`WorkspaceRoot`, `WorkspaceModules`) go in
  `tests/workspace_test.go`.

## Stage 1 — workspace root (new file `workspace.go`)

Create `workspace.go` with this content, moved verbatim in behaviour from
`https://github.com/webtyp/devflow/blob/main/workspace.go`:

```go
// workspaceFileExt is the marker that makes a directory a workspace root.
const workspaceFileExt = ".code-workspace"

// WorkspaceRoot returns the OUTERMOST ancestor of dir (dir included) that
// contains a *.code-workspace file, never climbing to $HOME or above.
// It returns "" when no ancestor qualifies.
func WorkspaceRoot(dir string) string {
	home, _ := os.UserHomeDir() // "" on error: then only the disk root stops the climb
	/* exact body of devflow.WorkspaceRootFrom, below */
}
```

devflow exported `WorkspaceRootFrom(dir, home)` and `WorkspaceFileExt` only for its own tests and
internals. Neither is exported here: the home seam is `t.Setenv("HOME", …)` in the test.

Body (copy exactly; it continues after the `home` line above):

```go
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	if home != "" {
		if absHome, err := filepath.Abs(home); err == nil {
			home = absHome
		}
	}
	found := ""
	d := abs
	for {
		if d == home || d == filepath.Dir(d) {
			break
		}
		matches, _ := filepath.Glob(filepath.Join(d, "*"+workspaceFileExt))
		for _, match := range matches {
			if info, err := os.Stat(match); err == nil && !info.IsDir() {
				found = d
				break
			}
		}
		d = filepath.Dir(d)
	}
	return found
```

Test `tests/workspace_test.go` (`package modfind_test`): port `TestWorkspaceRootFrom` from
`https://github.com/webtyp/devflow/blob/main/test/workspace_test.go` as `TestWorkspaceRoot`. Build
the tree under `home := t.TempDir()` and call `t.Setenv("HOME", home)` (on Unix `os.UserHomeDir`
reads `$HOME`), then call `modfind.WorkspaceRoot(dir)`. The cases stay the same: an outer
`Project.code-workspace` and an inner `velty.code-workspace`, where the outer one wins; a dir with
no marker returns `""`; and a marker placed directly in `home` is never found, because the climb
stops at `$HOME`.

## Stage 2 — modules in the workspace (same file `workspace.go`)

```go
// workspaceSkipDirs never hold a module the developer edits.
var workspaceSkipDirs = []string{"node_modules", "vendor", "testdata", "_temp"}

// WorkspaceModules walks root and returns one Module per go.mod found, with
// Path read from its `module` line and Dir = LocalDir = the go.mod's directory.
// Hidden directories (name starting with ".") and workspaceSkipDirs are not
// entered; root itself is always entered. Version, IsMain, IsReplace and
// Indirect are left zero. Order: lexical by Dir (filepath.WalkDir order).
func WorkspaceModules(root string) ([]Module, error)
```

Rules:
- Read the module path by parsing the first line that starts with `module ` (after trimming
  leading spaces): drop any trailing `// comment`, trim spaces, then trim surrounding `"`. Do NOT
  add `golang.org/x/mod` or any other dependency for this.
- A `go.mod` with no `module` line is skipped.
- An error reading one `go.mod` skips that file. Only a failure to start the walk returns an error.

Tests in `tests/workspace_test.go`:
1. Tree with `a/go.mod` (`module example.com/a`), `b/sub/go.mod` (`module example.com/b/sub`),
   `node_modules/x/go.mod`, `.hidden/y/go.mod`, `a/testdata/z/go.mod`, `_temp/w/go.mod` → exactly
   `example.com/a` and `example.com/b/sub`, with `Dir == LocalDir == ` their absolute dirs.
2. `module "example.com/q" // comment` → Path `example.com/q`.

## Stage 3 — `Module.LocalDir` and `SourceDir()` (`modfind.go`, `discover.go`)

Add to `Module` in `modfind.go`:

```go
	LocalDir  string // the developer's own checkout of Path: the main module, a local replace, or the
	                 // unique go.mod under WorkspaceRoot(rootDir) declaring Path; "" when none
```

and the method:

```go
// SourceDir is where a person reads and edits this module's code: LocalDir
// when the developer has a checkout, else Dir (possibly the read-only cache).
func (m Module) SourceDir() string
```

In `Finder.Discover`, after `parse` and **before** caching, fill `LocalDir` (new unexported
`func fillLocalDirs(rootDir string, mods []Module, log func(...any))` in `discover.go`):

1. `IsMain || IsReplace` → `LocalDir = Dir`.
2. Otherwise, call `root := WorkspaceRoot(rootDir)`. If `root == ""`, leave every `LocalDir` empty.
3. Else call `WorkspaceModules(root)` **once** and index it by Path. For each remaining module:
   - exactly one checkout declares its Path → `LocalDir` = that Dir;
   - several → take the one with the fewest path separators; if two tie at that depth, leave
     `LocalDir` empty and call `log` with
     `modfind: ambiguous local checkout for <Path>: <dir1>, <dir2>` (built with `fmt.Sprint`-style
     args through `f.log`, like the existing warnings).
4. A `WorkspaceModules` error → `log("modfind: workspace walk failed in", root, ":", err)` and
   leave `LocalDir` empty (Discover still succeeds).

`Seed` and `Dirs` are not touched by this plan. Their only remaining users after phase E are none:
`Seed` is called only by tests in other repos (forbidden API-for-tests), and `Dirs` is
`Discover` plus `m.Dir`. Both are deleted in master-plan phase G, after the consumers migrate to
`Discoverer`. Do NOT delete them here, or the consumers' builds break before they can migrate.

Tests (root `discover_test.go`, canned runner via the private `f.run`, as the existing tests do):
1. Build under `t.TempDir()`: `ws/Project.code-workspace`, `ws/app/go.mod` (`module example.com/app`),
   `ws/lib/go.mod` (`module example.com/lib`). Canned `go list` output: main `example.com/app` with
   `Dir` = `ws/app`; dep `example.com/lib` with `Dir` = some fake cache dir; dep `example.com/other`
   with a fake cache dir. Expect `lib.LocalDir == ws/lib`, `lib.SourceDir() == ws/lib`,
   `other.LocalDir == ""`, `other.SourceDir() == other.Dir`, `app.LocalDir == ws/app`.
2. Same, plus `ws/x/lib/go.mod` and `ws/y/lib/go.mod` both declaring `example.com/lib` and no
   shallower one → `LocalDir == ""` and the log sink received a line containing
   `ambiguous local checkout`.
3. No `*.code-workspace` anywhere above the temp tree → all deps `LocalDir == ""`.

## Stage 3b — the `Discoverer` contract (`modfind.go`)

Consumers (`webtyp/sitec`, `webtyp/ormc`, `webtyp/image`, `webtyp/app`) store a concrete
`*modfind.Finder`, which is why their tests needed `Seed`. They must depend on the contract instead
(api-design harness rule 9: logic depends on contracts; one composition root knows the concrete
type):

```go
// Discoverer is what tooling needs from module discovery: the modules visible
// from rootDir. *Finder is the real implementation; a consumer's test passes
// its own fake.
type Discoverer interface {
	Discover(rootDir string) ([]Module, error)
}

var _ Discoverer = (*Finder)(nil)
```

No test in this repo needs it: `Finder` is tested directly.

## Stage 4 — docs

- `README.md`: add `LocalDir` to the Module table and a "Local checkouts" usage block
  (`m.SourceDir()`, `modfind.WorkspaceRoot`, `modfind.WorkspaceModules`). Add a block "Inject the
  contract, not the Finder": consumers take a `modfind.Discoverer`; the composition root builds
  the `*Finder`; tests pass a fake.
- `docs/ARCHITECTURE.md`: one section "Local checkout resolution" describing Stage 3's rules
  (main/replace → Dir; unique workspace go.mod; shallowest wins; tie → empty + warning).

## Acceptance

- `gotest` passes.
- `grep -rn "TODO\|FIXME" --include='*.go' .` → empty.
- `go.mod` gained no new `require`.

## Stages

| # | Stage | Files |
|---|---|---|
| 0 | Test layout | `discover_test.go` (justification comment) |
| 1 | Workspace root | `workspace.go`, `tests/workspace_test.go` |
| 2 | WorkspaceModules | `workspace.go`, `tests/workspace_test.go` |
| 3 | LocalDir / SourceDir | `modfind.go`, `discover.go`, `discover_test.go` |
| 3b | Discoverer | `modfind.go` |
| 4 | Docs | `README.md`, `docs/ARCHITECTURE.md` |
