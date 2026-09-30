package transcript

import (
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Normalize makes paths comparable: absolute, forward slashes, and lowercase
// on Windows and macOS, whose default filesystems ignore case.
func Normalize(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	p = filepath.ToSlash(filepath.Clean(p))
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return p
}

// Match reports whether a written path touches target: the same file, a
// directory containing it, or a glob pattern matching it.
func Match(written, target string) bool {
	w, t := Normalize(written), Normalize(target)
	if w == t || strings.HasPrefix(t, strings.TrimSuffix(w, "/")+"/") {
		return true
	}
	if strings.ContainsAny(written, "*?[") {
		ok, _ := path.Match(w, t)
		return ok
	}
	return false
}
