// Root-level test (justified): Discover must be tested without the Go toolchain, which requires
// replacing the unexported go-list runner (f.run). Exporting a runner only for tests is forbidden
// by the ecosystem rule, and no consumer needs another runner.
package modfind

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// canned `go list -m -json all` output: a main module, a cache dep, a pruned
// dep (no Dir), and a local replace.
const cannedJSON = `{
	"Path": "webtyp.com/example",
	"Main": true,
	"Dir": "/home/u/dev/example",
	"GoVersion": "1.25.2"
}
{
	"Path": "github.com/some/dep",
	"Version": "v1.2.0",
	"Indirect": true,
	"Dir": "/home/u/go/pkg/mod/github.com/some/dep@v1.2.0"
}
{
	"Path": "github.com/pruned/dep",
	"Version": "v0.1.0",
	"Indirect": true
}
{
	"Path": "github.com/veltylabs/item-catalog",
	"Version": "v0.0.2",
	"Dir": "/home/u/go/pkg/mod/github.com/veltylabs/item-catalog@v0.0.2",
	"Replace": {
		"Path": "../modules/item-catalog",
		"Dir": "/home/u/dev/modules/item-catalog"
	}
}`

func fakeFinder() *Finder {
	f := New()
	f.run = func(string) ([]byte, error) { return []byte(cannedJSON), nil }
	return f
}

func TestDiscoverClassification(t *testing.T) {
	f := fakeFinder()
	mods, err := f.Discover("/root")
	if err != nil {
		t.Fatal(err)
	}
	// pruned dep (no Dir) must be skipped → 3 of 4 records.
	if len(mods) != 3 {
		t.Fatalf("want 3 modules, got %d: %+v", len(mods), mods)
	}

	by := map[string]Module{}
	for _, m := range mods {
		by[m.Path] = m
	}

	main := by["webtyp.com/example"]
	if !main.IsMain || !main.Writable() {
		t.Errorf("main module not classified writable: %+v", main)
	}

	cache := by["github.com/some/dep"]
	if cache.IsMain || cache.IsReplace || cache.Writable() {
		t.Errorf("cache dep should be read-only: %+v", cache)
	}

	repl := by["github.com/veltylabs/item-catalog"]
	if !repl.IsReplace || !repl.Writable() {
		t.Errorf("local replace not classified writable: %+v", repl)
	}
	if repl.Dir != "/home/u/dev/modules/item-catalog" {
		t.Errorf("replace must use the replacement Dir, got %q", repl.Dir)
	}
}

func TestDiscoverCachesAndRefresh(t *testing.T) {
	f := New()
	calls := 0
	f.run = func(string) ([]byte, error) { calls++; return []byte(cannedJSON), nil }

	_, _ = f.Discover("/root")
	_, _ = f.Discover("/root")
	if calls != 1 {
		t.Fatalf("expected 1 go list call (cached), got %d", calls)
	}

	f.Refresh("/root")
	_, _ = f.Discover("/root")
	if calls != 2 {
		t.Fatalf("expected re-run after Refresh, got %d calls", calls)
	}
}

func TestDirs(t *testing.T) {
	f := fakeFinder()
	dirs, err := f.Dirs("/root")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 3 {
		t.Fatalf("want 3 dirs, got %d", len(dirs))
	}
}

func TestSeedBypassesGoList(t *testing.T) {
	f := New()
	f.run = func(string) ([]byte, error) { t.Fatal("go list must not run after Seed"); return nil, nil }
	f.Seed("/root", []Module{{Path: "x", Dir: "/x", IsMain: true}})
	mods, err := f.Discover("/root")
	if err != nil || len(mods) != 1 || !mods[0].Writable() {
		t.Fatalf("seed not honored: %+v err=%v", mods, err)
	}
}

func TestLocalDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeMod := func(path, content string) {
		full := filepath.Join(home, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Case 1: normal local checkouts
	writeMod("ws/Project.code-workspace", "")
	writeMod("ws/app/go.mod", "module example.com/app\n")
	writeMod("ws/lib/go.mod", "module example.com/lib\n")

	canned := `{
		"Path": "example.com/app",
		"Main": true,
		"Dir": "` + filepath.Join(home, "ws/app") + `"
	}
	{
		"Path": "example.com/lib",
		"Version": "v1.0.0",
		"Dir": "/fake/cache/lib"
	}
	{
		"Path": "example.com/other",
		"Version": "v1.0.0",
		"Dir": "/fake/cache/other"
	}`

	f := New()
	f.run = func(string) ([]byte, error) { return []byte(canned), nil }
	mods, err := f.Discover(filepath.Join(home, "ws/app"))
	if err != nil {
		t.Fatal(err)
	}

	by := map[string]Module{}
	for _, m := range mods {
		by[m.Path] = m
	}

	lib := by["example.com/lib"]
	if lib.LocalDir != filepath.Join(home, "ws/lib") {
		t.Errorf("lib.LocalDir = %q, want %q", lib.LocalDir, filepath.Join(home, "ws/lib"))
	}
	if lib.SourceDir() != filepath.Join(home, "ws/lib") {
		t.Errorf("lib.SourceDir() = %q, want %q", lib.SourceDir(), filepath.Join(home, "ws/lib"))
	}

	other := by["example.com/other"]
	if other.LocalDir != "" {
		t.Errorf("other.LocalDir = %q, want empty", other.LocalDir)
	}
	if other.SourceDir() != "/fake/cache/other" {
		t.Errorf("other.SourceDir() = %q, want %q", other.SourceDir(), "/fake/cache/other")
	}

	app := by["example.com/app"]
	if app.LocalDir != filepath.Join(home, "ws/app") {
		t.Errorf("app.LocalDir = %q, want %q", app.LocalDir, filepath.Join(home, "ws/app"))
	}

	// Case 2: ambiguous local checkouts
	writeMod("ws/x/lib/go.mod", "module example.com/lib\n")
	writeMod("ws/y/lib/go.mod", "module example.com/lib\n")
	// remove the unambiguous one to force a tie
	os.Remove(filepath.Join(home, "ws/lib/go.mod"))

	f2 := New()
	f2.run = func(string) ([]byte, error) { return []byte(canned), nil }

	var logLines []string
	f2.SetLog(func(args ...any) {
		var s []string
		for _, a := range args {
			s = append(s, fmt.Sprint(a))
		}
		logLines = append(logLines, strings.Join(s, ""))
	})

	mods2, err := f2.Discover(filepath.Join(home, "ws/app"))
	if err != nil {
		t.Fatal(err)
	}

	by2 := map[string]Module{}
	for _, m := range mods2 {
		by2[m.Path] = m
	}

	lib2 := by2["example.com/lib"]
	if lib2.LocalDir != "" {
		t.Errorf("lib2.LocalDir = %q, want empty (ambiguous)", lib2.LocalDir)
	}

	foundWarning := false
	for _, l := range logLines {
		if strings.Contains(l, "ambiguous local checkout") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Errorf("expected ambiguous local checkout warning, got logs: %v", logLines)
	}

	// Case 3: No *.code-workspace
	home2 := t.TempDir()
	t.Setenv("HOME", home2)
	writeMod2 := func(path, content string) {
		full := filepath.Join(home2, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeMod2("app/go.mod", "module example.com/app\n")
	writeMod2("lib/go.mod", "module example.com/lib\n")

	canned2 := `{
		"Path": "example.com/app",
		"Main": true,
		"Dir": "` + filepath.Join(home2, "app") + `"
	}
	{
		"Path": "example.com/lib",
		"Version": "v1.0.0",
		"Dir": "/fake/cache/lib"
	}`

	f3 := New()
	f3.run = func(string) ([]byte, error) { return []byte(canned2), nil }
	mods3, err := f3.Discover(filepath.Join(home2, "app"))
	if err != nil {
		t.Fatal(err)
	}

	by3 := map[string]Module{}
	for _, m := range mods3 {
		by3[m.Path] = m
	}

	lib3 := by3["example.com/lib"]
	if lib3.LocalDir != "" {
		t.Errorf("lib3.LocalDir = %q, want empty (no workspace)", lib3.LocalDir)
	}
}
