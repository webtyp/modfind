package modfind

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// workspaceFileExt is the marker that makes a directory a workspace root.
const workspaceFileExt = ".code-workspace"

// WorkspaceRoot returns the OUTERMOST ancestor of dir (dir included) that
// contains a *.code-workspace file, never climbing to $HOME or above.
// It returns "" when no ancestor qualifies.
func WorkspaceRoot(dir string) string {
	home, _ := os.UserHomeDir() // "" on error: then only the disk root stops the climb
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
}

// workspaceSkipDirs never hold a module the developer edits.
var workspaceSkipDirs = []string{"node_modules", "vendor", "testdata", "_temp"}

// WorkspaceModules walks root and returns one Module per go.mod found, with
// Path read from its `module` line and Dir = LocalDir = the go.mod's directory.
// Hidden directories (name starting with ".") and workspaceSkipDirs are not
// entered; root itself is always entered. Version, IsMain, IsReplace and
// Indirect are left zero. Order: lexical by Dir (filepath.WalkDir order).
func WorkspaceModules(root string) ([]Module, error) {
	var mods []Module
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err // only failure to start the walk returns an error
			}
			return nil
		}

		if d.IsDir() {
			if path == root {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			for _, skip := range workspaceSkipDirs {
				if name == skip {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if d.Name() == "go.mod" {
			f, err := os.Open(path)
			if err != nil {
				return nil // error reading one go.mod skips that file
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(line, "module ") {
					pathVal := line[len("module "):]
					// drop any trailing // comment
					if idx := strings.Index(pathVal, "//"); idx != -1 {
						pathVal = pathVal[:idx]
					}
					pathVal = strings.TrimSpace(pathVal)
					pathVal = strings.Trim(pathVal, `"`)

					if pathVal != "" {
						dir := filepath.Dir(path)
						mods = append(mods, Module{
							Path:     pathVal,
							Dir:      dir,
							LocalDir: dir,
						})
					}
					break
				}
			}
		}
		return nil
	})

	return mods, err
}
