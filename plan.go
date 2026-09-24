package main

import (
	"os"
	"path/filepath"
	"sort"
)

// applyPlan performs removes outside the skill dir first, then copies, then
// removes inside it — so a file never disappears before its replacement lands.
func applyPlan(plan *Plan, dryRun bool) {
	if dryRun {
		return
	}
	skillRoot := resolve(plan.Target.SkillDir)
	var outside, inside []Action
	for _, action := range plan.Actions {
		if action.Kind != "remove" {
			continue
		}
		if !isInside(resolve(action.Path), skillRoot) || resolve(action.Path) == skillRoot {
			outside = append(outside, action)
		} else {
			inside = append(inside, action)
		}
	}
	for _, action := range removalOrder(outside) {
		if present(action.Path) {
			trashPath(action.Path)
		}
	}
	for _, action := range plan.Actions {
		if action.Kind != "copy" {
			continue
		}
		dest := action.Path
		if isSymlink(dest) {
			trashPath(dest)
		} else if isDir(dest) {
			die("destination is a directory: %s", display(dest))
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			die("cannot create %s: %v", display(filepath.Dir(dest)), err)
		}
		copyFile(action.Src, dest)
	}
	for _, action := range removalOrder(inside) {
		if present(action.Path) {
			trashPath(action.Path)
		}
	}
}

// removalOrder removes files before directories, deepest directories first.
func removalOrder(actions []Action) []Action {
	var files, dirs []Action
	for _, a := range actions {
		if isSymlink(a.Path) || !isDir(a.Path) {
			files = append(files, a)
		} else {
			dirs = append(dirs, a)
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return pathDepth(dirs[i].Path) > pathDepth(dirs[j].Path)
	})
	return append(files, dirs...)
}

func pathDepth(path string) int {
	n := 0
	for p := resolve(path); ; {
		parent := filepath.Dir(p)
		if parent == p {
			return n
		}
		n++
		p = parent
	}
}
