package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// userError is a user-facing failure. Its message is printed as-is.
type userError struct{ msg string }

func (e *userError) Error() string { return e.msg }

func die(format string, args ...any) {
	panic(&userError{fmt.Sprintf(format, args...)})
}

func expandHome(path string) string {
	if path == "~" {
		return homeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir(), path[2:])
	}
	return path
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		die("cannot determine home directory")
	}
	return h
}

// resolve approximates Python's Path.resolve(strict=False): it follows
// symlinks on the deepest existing ancestor and cleans the rest.
func resolve(path string) string {
	abs, err := filepath.Abs(expandHome(path))
	if err != nil {
		return filepath.Clean(expandHome(path))
	}
	var tail []string
	cur := abs
	for {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				r = filepath.Join(r, tail[i])
			}
			return r
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

func isInside(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func isSymlink(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

func present(path string) bool {
	return isSymlink(path) || exists(path)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileMtime(path string) (time.Time, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	return st.ModTime(), true
}

// displayLink shows a path as stored, without following a symlink at the end.
func displayLink(path string) string {
	expanded := expandHome(path)
	rel, err := filepath.Rel(homeDir(), expanded)
	if err != nil || strings.HasPrefix(rel, "..") {
		return expanded
	}
	if rel == "." {
		return "~"
	}
	return "~/" + filepath.ToSlash(rel)
}

// display shows a resolved path relative to home when possible.
func display(path string) string {
	resolved := resolve(path)
	rel, err := filepath.Rel(resolve(homeDir()), resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		return resolved
	}
	if rel == "." {
		return "~"
	}
	return "~/" + filepath.ToSlash(rel)
}

// copyFile replicates shutil.copy2: data, mode, and modification time.
// The destination is replaced via rename, never rewritten in place: macOS
// kills executables whose contents change after creation ("Code Signature
// Invalid" at exec), so the copy must produce a fresh inode.
func copyFile(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		die("cannot read %s: %v", display(src), err)
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		die("cannot stat %s: %v", display(src), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".*")
	if err != nil {
		die("cannot write %s: %v", display(dst), err)
	}
	tmpName := tmp.Name()
	fail := func(err error) {
		tmp.Close()
		os.Remove(tmpName)
		die("cannot copy to %s: %v", display(dst), err)
	}
	if _, err := tmp.ReadFrom(in); err != nil {
		fail(err)
	}
	if err := tmp.Chmod(st.Mode()); err != nil {
		fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		die("cannot write %s: %v", display(dst), err)
	}
	os.Chtimes(tmpName, time.Now(), st.ModTime())
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		die("cannot write %s: %v", display(dst), err)
	}
}

func atomicWrite(path, text string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		die("cannot create %s: %v", display(filepath.Dir(path)), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		die("cannot write %s: %v", display(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		die("cannot write %s: %v", display(path), err)
	}
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		die("cannot read %s: %v", display(path), err)
	}
	return string(data)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func uniqueStrings(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}

func contains(items []string, item string) bool {
	for _, i := range items {
		if i == item {
			return true
		}
	}
	return false
}
