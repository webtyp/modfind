package modfind_test

import (
	"os"
	"path/filepath"
	"testing"

	"webtyp.com/modfind"
)

func TestWorkspaceRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// On Unix os.UserHomeDir reads $HOME. We don't want to test the OS specifically,
	// just the logic that uses it.

	// Outer project
	outer := filepath.Join(home, "outer")
	if err := os.MkdirAll(outer, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outer, "Project.code-workspace"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	// Inner project
	inner := filepath.Join(outer, "inner")
	if err := os.MkdirAll(inner, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "velty.code-workspace"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	// Subdir inside inner
	subdir := filepath.Join(inner, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	// No marker dir
	noMarker := filepath.Join(home, "nomarker")
	if err := os.MkdirAll(noMarker, 0755); err != nil {
		t.Fatal(err)
	}

	// Marker in home
	if err := os.WriteFile(filepath.Join(home, "home.code-workspace"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		dir  string
		want string
	}{
		{
			name: "outer marker wins for inner dir",
			dir:  subdir,
			want: outer,
		},
		{
			name: "outer marker wins for outer dir",
			dir:  outer,
			want: outer,
		},
		{
			name: "no marker",
			dir:  noMarker,
			want: "",
		},
		{
			name: "marker in home is ignored",
			dir:  home,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := modfind.WorkspaceRoot(tt.dir)
			if got != tt.want {
				t.Errorf("WorkspaceRoot(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

func TestWorkspaceModules(t *testing.T) {
	root := t.TempDir()

	writeMod := func(path, content string) {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	writeMod("a/go.mod", "module example.com/a\n")
	writeMod("b/sub/go.mod", "module example.com/b/sub\n")
	writeMod("node_modules/x/go.mod", "module example.com/x\n")
	writeMod(".hidden/y/go.mod", "module example.com/y\n")
	writeMod("a/testdata/z/go.mod", "module example.com/z\n")
	writeMod("_temp/w/go.mod", "module example.com/w\n")
	writeMod("c/go.mod", `module "example.com/q" // comment`+"\n")
	writeMod("empty/go.mod", "\n\n")

	mods, err := modfind.WorkspaceModules(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(mods) != 3 {
		t.Fatalf("got %d modules, want 3 (a, b/sub, c): %+v", len(mods), mods)
	}

	// Verify order is lexical by Dir
	wants := []struct {
		path string
		dir  string
	}{
		{"example.com/a", filepath.Join(root, "a")},
		{"example.com/b/sub", filepath.Join(root, "b/sub")},
		{"example.com/q", filepath.Join(root, "c")},
	}

	for i, want := range wants {
		if mods[i].Path != want.path {
			t.Errorf("mods[%d].Path = %q, want %q", i, mods[i].Path, want.path)
		}
		if mods[i].Dir != want.dir {
			t.Errorf("mods[%d].Dir = %q, want %q", i, mods[i].Dir, want.dir)
		}
		if mods[i].LocalDir != want.dir {
			t.Errorf("mods[%d].LocalDir = %q, want %q", i, mods[i].LocalDir, want.dir)
		}
	}
}
