// Package modfind is the centralized Go module discovery primitive for webtyp
// tooling. It runs `go list -m -json all` once per project root, caches the
// parsed result, and classifies each module as writable (main module or local
// replace — tooling may generate files there) or read-only (module cache).
//
// It replaces the byte-identical `go list -m -json all` loops previously
// copy-pasted in ssr, image/min and imagemin. Sibling to depfind: depfind maps
// the package import graph; modfind enumerates modules.
package modfind

// Discoverer is what tooling needs from module discovery: the modules visible
// from rootDir. *Finder is the real implementation; a consumer's test passes
// its own fake.
type Discoverer interface {
	Discover(rootDir string) ([]Module, error)
}

var _ Discoverer = (*Finder)(nil)

// Module is one Go module on disk, classified for tooling.
type Module struct {
	Path      string // import path, e.g. "github.com/veltylabs/item-catalog"
	Dir       string // absolute on-disk dir (cache path or local replace path)
	Version   string // empty for Main and for local replace targets
	IsMain    bool   // the root module of the project at rootDir
	IsReplace bool   // satisfied by a local (filesystem) replace directive
	Indirect  bool   // transitive dependency (not a direct require)
	LocalDir  string // the developer's own checkout of Path: the main module, a local replace, or the
	                 // unique go.mod under WorkspaceRoot(rootDir) declaring Path; "" when none
}

// Writable reports whether tooling may generate files inside Dir. True for the
// main module and for local replace targets; false for the read-only module
// cache.
func (m Module) Writable() bool { return m.IsMain || m.IsReplace }

// SourceDir is where a person reads and edits this module's code: LocalDir
// when the developer has a checkout, else Dir (possibly the read-only cache).
func (m Module) SourceDir() string {
	if m.LocalDir != "" {
		return m.LocalDir
	}
	return m.Dir
}
