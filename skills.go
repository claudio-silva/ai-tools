package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// catalog is a resolved source repo's discovered tools.
type catalog struct {
	src    *Source
	root   string
	skills []Skill
	mcps   []*McpServer
}

var catalogs = map[string]*catalog{}

// catalogFor discovers skills and MCPs in a source, fetching remote sources on
// first use. Results are memoized for the duration of the command.
func catalogFor(src *Source) *catalog {
	if cat, ok := catalogs[src.URL]; ok {
		return cat
	}
	root := src.root()
	skills, mcps := discover(root, src.URL)
	if len(skills) == 0 && len(mcps) == 0 {
		die("%s has no tools: no SKILL.md skills and no MCP servers (aitools.json)", src.URL)
	}
	cat := &catalog{src: src, root: root, skills: skills, mcps: mcps}
	catalogs[src.URL] = cat
	return cat
}

func relIn(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

type manifestEntry struct {
	from, to  string   // to == "" for plain MCP paths, or a skill's default placement
	platforms []string // skill files only; nil = every platform the skill supports
}

// manifest is a skill's optional aitools.json. Without one, the skill
// directory is copied whole to every platform.
type manifest struct {
	platforms []string // nil = every platform
	exclude   []string
	install   []manifestEntry
	remove    []string
}

func variablesFor(target Target) map[string]string {
	return map[string]string{
		"SKILL_DIR":     target.SkillDir,
		"SKILLS_DIR":    target.SkillsDir,
		"PLATFORM_HOME": target.PlatformHome,
		"CODEX_HOME":    codexHome(),
		"HOME":          homeDir(),
	}
}

func expandVars(text string, vars map[string]string, where string) string {
	out := text
	for _, token := range variables {
		out = strings.ReplaceAll(out, "$"+token, vars[token])
	}
	if strings.Contains(out, "$") {
		for _, token := range strings.Split(strings.ReplaceAll(text, "$$", ""), "$")[1:] {
			var name strings.Builder
			for _, ch := range token {
				if ch == '_' || ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') {
					name.WriteRune(ch)
				} else {
					break
				}
			}
			ident := name.String()
			if ident != "" && !contains(variables, ident) {
				die("%s: unknown variable $%s", where, ident)
			}
		}
	}
	trailingSlash := strings.HasSuffix(out, "/")
	expanded := expandHome(out)
	if !filepath.IsAbs(expanded) {
		die("%s: %s must expand to an absolute path", where, text)
	}
	rendered := filepath.Clean(expanded)
	if trailingSlash && !strings.HasSuffix(rendered, "/") {
		rendered += "/"
	}
	return rendered
}

func allowedRoots(target Target) []string {
	h := homeDir()
	roots := []string{
		target.SkillsDir,
		target.PlatformHome,
		codexHome(),
		filepath.Join(h, ".cursor"),
		filepath.Join(h, ".codex"),
		filepath.Join(h, ".claude"),
		filepath.Join(h, ".config", "devin"),
		filepath.Join(h, ".agents"),
	}
	if target.Project != "" {
		roots = append(roots, target.Project)
	}
	return roots
}

func protectedPaths(target Target) map[string]bool {
	c := codexHome()
	h := homeDir()
	paths := []string{
		target.SkillsDir,
		target.PlatformHome,
		c,
		filepath.Join(c, "agents"),
		filepath.Join(c, "skills"),
		h,
		filepath.Join(h, ".cursor"),
		filepath.Join(h, ".cursor", "skills"),
		filepath.Join(h, ".codex"),
		filepath.Join(h, ".claude"),
		filepath.Join(h, ".claude", "skills"),
		filepath.Join(h, ".config", "devin"),
		filepath.Join(h, ".config", "devin", "skills"),
		filepath.Join(h, ".agents"),
		filepath.Join(h, ".agents", "skills"),
	}
	if target.Project != "" {
		p := target.Project
		paths = append(paths,
			p,
			filepath.Join(p, ".cursor"),
			filepath.Join(p, ".cursor", "skills"),
			filepath.Join(p, ".codex"),
			filepath.Join(p, ".agents"),
			filepath.Join(p, ".agents", "skills"),
			filepath.Join(p, ".claude"),
			filepath.Join(p, ".claude", "skills"),
			filepath.Join(p, ".devin"),
			filepath.Join(p, ".devin", "skills"),
		)
	}
	set := map[string]bool{}
	for _, p := range paths {
		set[resolve(p)] = true
	}
	return set
}

func resolveAllowed(path string, target Target, purpose string) string {
	resolved := resolve(path)
	if protectedPaths(target)[resolved] {
		die("%s %s is a protected directory", purpose, display(resolved))
	}
	for _, root := range allowedRoots(target) {
		rootResolved := resolve(root)
		if isInside(resolved, rootResolved) && resolved != rootResolved {
			return resolved
		}
	}
	die("%s %s is outside the install directories for %s (%s)", purpose, display(resolved), target.Platform, target.Scope)
	return ""
}

func sourceOK(skillName, relative string, glob bool) {
	if relative == "" || strings.HasPrefix(relative, "/") || strings.HasPrefix(relative, "~") {
		die("%s: source must stay inside the skill directory (%s)", skillName, relative)
	}
	parts := strings.Split(relative, "/")
	if contains(parts, "..") {
		die("%s: source must not contain .. (%s)", skillName, relative)
	}
	if strings.Contains(relative, "**") {
		die("%s: ** globs are not allowed (%s)", skillName, relative)
	}
	hasGlob := false
	for _, part := range parts {
		if strings.ContainsAny(part, "*?") {
			hasGlob = true
		}
	}
	if glob && !hasGlob {
		die("%s: expected a glob such as dir/*.toml (%s)", skillName, relative)
	}
	if !glob && hasGlob {
		die("%s: unexpected glob in %s", skillName, relative)
	}
}

func ignoredFile(rel string) bool {
	base := filepath.Base(rel)
	if base == ".DS_Store" || strings.HasSuffix(base, ".pyc") {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if skipDirNames[part] || strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func filesUnder(skillName, skillPath, src string) []string {
	if isFile(src) {
		return []string{src}
	}
	if !isDir(src) {
		rel, _ := filepath.Rel(skillPath, src)
		die("%s: not a file or directory: %s", skillName, rel)
	}
	var found []string
	filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != src && (skipDirNames[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		if ignoredFile(rel) || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		found = append(found, path)
		return nil
	})
	if len(found) == 0 {
		rel, _ := filepath.Rel(skillPath, src)
		die("%s: %s has no files to install", skillName, rel)
	}
	return found
}

func hasGlobChars(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if strings.ContainsAny(part, "*?") {
			return true
		}
	}
	return false
}

type filePair struct{ src, dest string }

// entryTo is a files entry's destination; without "to" the source keeps its
// relative path inside the destination skill directory.
func entryTo(entry manifestEntry) string {
	if entry.to != "" {
		return entry.to
	}
	if hasGlobChars(entry.from) {
		dir := path.Dir(entry.from)
		if dir == "." {
			return "$SKILL_DIR/"
		}
		return "$SKILL_DIR/" + dir + "/"
	}
	return "$SKILL_DIR/" + entry.from
}

// entrySources resolves a files entry's source: the matched files of a glob,
// the files under a directory, or the single file.
func entrySources(skill Skill, entry manifestEntry) (kind string, src string, files []string) {
	if hasGlobChars(entry.from) {
		sourceOK(skill.Name, entry.from, true)
		matches, _ := filepath.Glob(filepath.Join(skill.Path, entry.from))
		for _, match := range matches {
			if isFile(match) {
				rel, _ := filepath.Rel(skill.Path, match)
				if !ignoredFile(rel) {
					files = append(files, match)
				}
			}
		}
		if len(files) == 0 {
			die("%s: no files matched %s", skill.Name, entry.from)
		}
		sort.Strings(files)
		return "glob", "", files
	}
	sourceOK(skill.Name, entry.from, false)
	src = filepath.Join(skill.Path, entry.from)
	if !exists(src) {
		die("%s: missing %s", skill.Name, entry.from)
	}
	if isDir(src) {
		return "dir", src, filesUnder(skill.Name, skill.Path, src)
	}
	return "file", src, []string{src}
}

func excluded(rel string, patterns []string) bool {
	for _, pattern := range patterns {
		p := strings.TrimSuffix(pattern, "/")
		for c := rel; c != "." && c != "/"; c = path.Dir(c) {
			if ok, _ := path.Match(p, c); ok || c == p {
				return true
			}
		}
	}
	return false
}

// defaultFiles lists the files copied as-is into the skill directory: all of
// it except junk, aitools.json, exclude matches, and files entries' sources.
func defaultFiles(skill Skill, m manifest) []string {
	placed := map[string]bool{}
	for _, entry := range m.install {
		_, _, files := entrySources(skill, entry)
		for _, f := range files {
			placed[f] = true
		}
	}
	var out []string
	filepath.WalkDir(skill.Path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != skill.Path && (skipDirNames[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(skill.Path, p)
		rel = filepath.ToSlash(rel)
		if ignoredFile(rel) || rel == manifestName || placed[p] || excluded(rel, m.exclude) || !d.Type().IsRegular() {
			return nil
		}
		out = append(out, p)
		return nil
	})
	return out
}

func expandInstall(skill Skill, m manifest, target Target) []filePair {
	vars := variablesFor(target)
	var pairs []filePair
	seen := map[string]string{}

	add := func(src, dest string) {
		resolvedSrc := resolve(src)
		if !isInside(resolvedSrc, resolve(skill.Path)) {
			die("%s: source escapes the skill directory (%s)", skill.Name, src)
		}
		resolvedDest := resolveAllowed(dest, target, skill.Name+" destination")
		if prev, ok := seen[resolvedDest]; ok && prev != resolvedSrc {
			die("%s: %s is installed twice", skill.Name, display(resolvedDest))
		}
		seen[resolvedDest] = resolvedSrc
		pairs = append(pairs, filePair{resolvedSrc, resolvedDest})
	}

	for _, file := range defaultFiles(skill, m) {
		rel, _ := filepath.Rel(skill.Path, file)
		add(file, filepath.Join(target.SkillDir, rel))
	}
	for _, entry := range m.install {
		if entry.platforms != nil && !contains(entry.platforms, target.Platform) {
			continue
		}
		kind, src, files := entrySources(skill, entry)
		destText := expandVars(entryTo(entry), vars, skill.Name+" to")
		switch kind {
		case "glob":
			for _, file := range files {
				add(file, filepath.Join(destText, filepath.Base(file)))
			}
		case "dir":
			for _, file := range files {
				rel, _ := filepath.Rel(src, file)
				add(file, filepath.Join(destText, rel))
			}
		default:
			dest := destText
			if strings.HasSuffix(destText, "/") || isDir(destText) {
				dest = filepath.Join(destText, filepath.Base(src))
			}
			add(src, dest)
		}
	}

	skillMD := filepath.Join(resolve(target.SkillDir), "SKILL.md")
	for _, p := range pairs {
		if p.dest == skillMD {
			return pairs
		}
	}
	die("%s: SKILL.md must be installed into the skill directory", skill.Name)
	return nil
}

func expandRemove(skillName string, m manifest, target Target) []string {
	vars := variablesFor(target)
	var paths []string
	for _, entry := range m.remove {
		text := expandVars(entry, vars, skillName+" remove")
		paths = append(paths, resolveAllowed(text, target, skillName+" remove"))
	}
	return paths
}

func staleFiles(skillDir string, keep map[string]bool) []string {
	if !isDir(skillDir) || isSymlink(skillDir) {
		return nil
	}
	var stale []string
	filepath.WalkDir(skillDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != skillDir && skipDirNames[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(skillDir, path)
		if !keep[filepath.ToSlash(rel)] {
			stale = append(stale, resolve(path))
		}
		return nil
	})
	return stale
}

func emptyDirs(skillDir string, keep map[string]bool) []string {
	if !isDir(skillDir) || isSymlink(skillDir) {
		return nil
	}
	var allDirs []string
	children := map[string][]string{}
	filepath.WalkDir(skillDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		allDirs = append(allDirs, path)
		children[filepath.Dir(path)] = append(children[filepath.Dir(path)], path)
		return nil
	})
	var doomed []string
	doomedSet := map[string]bool{}
	// deepest first
	sort.Slice(allDirs, func(i, j int) bool { return len(allDirs[i]) > len(allDirs[j]) })
	for _, path := range allDirs {
		if path == skillDir {
			continue
		}
		rel, _ := filepath.Rel(skillDir, path)
		relDir := filepath.ToSlash(rel)
		keepUnder := false
		for k := range keep {
			if strings.HasPrefix(k, relDir+"/") {
				keepUnder = true
				break
			}
		}
		living := 0
		for _, child := range children[path] {
			if !doomedSet[child] {
				living++
			}
		}
		if !keepUnder && living == 0 {
			doomed = append(doomed, path)
			doomedSet[path] = true
		}
	}
	return doomed
}

func directoryMtime(skillDir string) (time.Time, bool) {
	if !isDir(skillDir) || isSymlink(skillDir) {
		return time.Time{}, false
	}
	var latest time.Time
	filepath.WalkDir(skillDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != skillDir && (skipDirNames[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".DS_Store" || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		if st, err := os.Stat(path); err == nil && st.ModTime().After(latest) {
			latest = st.ModTime()
		}
		return nil
	})
	if latest.IsZero() {
		return time.Time{}, false
	}
	return latest, true
}

func repoMtime(skill Skill) time.Time {
	m := skill.Manifest
	files := defaultFiles(skill, m)
	for _, entry := range m.install {
		_, _, matched := entrySources(skill, entry)
		files = append(files, matched...)
	}
	var latest time.Time
	for _, p := range files {
		if st, err := os.Stat(p); err == nil && st.ModTime().After(latest) {
			latest = st.ModTime()
		}
	}
	if latest.IsZero() {
		die("%s: no files to version", skill.Name)
	}
	return latest
}

func installedContentMtime(skill Skill, target Target) (time.Time, bool) {
	pairs := expandInstall(skill, skill.Manifest, target)
	var latest time.Time
	for _, p := range pairs {
		if st, err := os.Stat(p.dest); err == nil && st.Mode().IsRegular() && st.ModTime().After(latest) {
			latest = st.ModTime()
		}
	}
	return latest, !latest.IsZero()
}

func repoIsNewer(skill Skill, target Target) bool {
	if !isFile(filepath.Join(target.SkillDir, "SKILL.md")) {
		return false
	}
	for _, p := range expandInstall(skill, skill.Manifest, target) {
		st, err := os.Stat(p.dest)
		srcSt, srcErr := os.Stat(p.src)
		if err != nil || !st.Mode().IsRegular() || (srcErr == nil && srcSt.ModTime().After(st.ModTime())) {
			return true
		}
	}
	return false
}

func parseFrontmatter(path string) map[string]string {
	text := readFile(path)
	meta := map[string]string{}
	if !strings.HasPrefix(text, "---\n") {
		return meta
	}
	end := strings.Index(text[3:], "\n---")
	if end == -1 {
		return meta
	}
	current := ""
	for _, line := range strings.Split(text[4:3+end], "\n") {
		if current != "" && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			extra := strings.TrimSpace(line)
			if extra != "" {
				meta[current] = strings.TrimSpace(meta[current] + " " + extra)
			}
			continue
		}
		idx := strings.Index(line, ":")
		if idx == -1 {
			current = ""
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if len(value) >= 2 && value[0] == value[len(value)-1] && (value[0] == '"' || value[0] == '\'') {
			value = value[1 : len(value)-1]
		}
		meta[key] = value
		current = key
	}
	return meta
}

func planInstall(skill Skill, target Target, data *stateData, skipKeys map[entryKey]bool, replace, force bool) *Plan {
	m := skill.Manifest
	if isSymlink(target.SkillDir) {
		die("%s is a symlink; refusing to install over it. Remove the link manually.", displayLink(target.SkillDir))
	}
	key := target.key(skill.Name)
	current := findEntry(data.Installs, key, "skill")
	foreign := present(target.SkillDir) && current == nil
	if foreign && !replace && !force {
		die("%s already exists and was not installed by this tool. Move it aside, then install again.", display(target.SkillDir))
	}
	if current != nil && resolve(current.SkillDir) != resolve(target.SkillDir) {
		die("recorded install path for %s does not match %s", skill.Name, display(target.SkillDir))
	}

	pairs := expandInstall(skill, m, target)
	removes := expandRemove(skill.Name, m, target)
	skillRoot := resolve(target.SkillDir)
	destSet := map[string]bool{}
	for _, p := range pairs {
		destSet[p.dest] = true
	}
	for _, path := range removes {
		overlap := path == skillRoot
		for dest := range destSet {
			if dest == path || isInside(dest, path) {
				overlap = true
			}
		}
		if overlap {
			die("%s: remove path overlaps an installed file: %s", skill.Name, display(path))
		}
	}

	inside := map[string]bool{}
	var external []string
	for dest := range destSet {
		if isInside(dest, skillRoot) {
			rel, _ := filepath.Rel(skillRoot, dest)
			inside[filepath.ToSlash(rel)] = true
		} else {
			external = append(external, dest)
		}
	}
	sort.Strings(external)

	plan := &Plan{Skill: skill, Target: target, ExternalFiles: external}
	if foreign && !replace {
		// install --force over an unmanaged directory: drop it wholesale before
		// the copies land, so no foreign files get mixed into the install. The
		// path stays unresolved so a symlink is trashed, not its target.
		plan.Actions = append(plan.Actions, Action{Kind: "remove", Path: target.SkillDir})
	}
	for _, path := range removes {
		if !present(path) {
			continue
		}
		if stillNeeded(data.Installs, path, skipKeys) {
			plan.Actions = append(plan.Actions, Action{Kind: "keep", Path: path, Note: "still used by another install"})
		} else {
			plan.Actions = append(plan.Actions, Action{Kind: "remove", Path: path})
		}
	}
	for _, p := range pairs {
		plan.Actions = append(plan.Actions, Action{Kind: "copy", Path: p.dest, Src: p.src})
	}
	if current != nil && !replace {
		for _, path := range staleFiles(target.SkillDir, inside) {
			if !destSet[path] {
				plan.Actions = append(plan.Actions, Action{Kind: "remove", Path: path})
			}
		}
		for _, path := range emptyDirs(target.SkillDir, inside) {
			plan.Actions = append(plan.Actions, Action{Kind: "remove", Path: path})
		}
		for _, item := range current.ExternalFiles {
			path := resolveAllowed(item, target, "cannot remove")
			if contains(external, path) {
				continue
			}
			if stillNeeded(data.Installs, path, skipKeys) {
				plan.Actions = append(plan.Actions, Action{Kind: "keep", Path: path, Note: "still used by another install"})
			} else if present(path) {
				plan.Actions = append(plan.Actions, Action{Kind: "remove", Path: path})
			}
		}
	}
	return plan
}

func planUninstall(skillName string, target Target, m *manifest, data *stateData, skipKeys map[entryKey]bool, force bool) *Plan {
	if isSymlink(target.SkillDir) {
		die("%s is a symlink; refusing to remove it. Remove the link manually.", displayLink(target.SkillDir))
	}
	current := findEntry(data.Installs, target.key(skillName), "skill")
	if current == nil {
		if !force || !present(target.SkillDir) {
			return nil
		}
		// --force: drop an unrecorded directory; we have no metadata about its
		// parts, so only the directory itself is removed. The path stays
		// unresolved so a symlink is trashed, not its target.
		return &Plan{
			Skill:   Skill{Name: skillName, Path: "."},
			Target:  target,
			Actions: []Action{{Kind: "remove", Path: target.SkillDir}},
		}
	}
	plan := &Plan{Skill: Skill{Name: skillName, Path: "."}, Target: target}
	if m != nil {
		for _, path := range expandRemove(skillName, *m, target) {
			if !present(path) {
				continue
			}
			if stillNeeded(data.Installs, path, skipKeys) {
				plan.Actions = append(plan.Actions, Action{Kind: "keep", Path: path, Note: "still used by another install"})
			} else {
				plan.Actions = append(plan.Actions, Action{Kind: "remove", Path: path})
			}
		}
	}
	for _, item := range current.ExternalFiles {
		// recorded paths are revalidated before removal — a tampered or
		// stale state file must not expand what this command can delete
		path := resolveAllowed(item, target, "cannot remove")
		if stillNeeded(data.Installs, path, skipKeys) {
			plan.Actions = append(plan.Actions, Action{Kind: "keep", Path: path, Note: "still used by another install"})
		} else if present(path) {
			plan.Actions = append(plan.Actions, Action{Kind: "remove", Path: path})
		}
	}
	skillDir := resolveAllowed(current.SkillDir, target, "cannot remove")
	if stillNeeded(data.Installs, skillDir, skipKeys) {
		plan.Actions = append(plan.Actions, Action{Kind: "keep", Path: skillDir, Note: "still used by another install"})
	} else if present(skillDir) {
		plan.Actions = append(plan.Actions, Action{Kind: "remove", Path: skillDir})
	}
	return plan
}

func platformsForSkill(skill Skill, filter []string, strict bool) []string {
	if filter == nil {
		return skill.Platforms
	}
	var unsupported []string
	for _, name := range filter {
		if !contains(skill.Platforms, name) {
			unsupported = append(unsupported, name)
		}
	}
	if len(unsupported) > 0 && strict {
		die("%s cannot target %s; it supports %s",
			skill.Name, strings.Join(unsupported, ", "), strings.Join(skill.Platforms, ", "))
	}
	var out []string
	for _, p := range skill.Platforms {
		if contains(filter, p) {
			out = append(out, p)
		}
	}
	return out
}

func skillsByName(skills []Skill) map[string]Skill {
	m := map[string]Skill{}
	for _, s := range skills {
		m[s.Name] = s
	}
	return m
}

func resolveRequested(skills []Skill, names []string, data *stateData) {
	known := map[string]bool{}
	for _, s := range skills {
		known[s.Name] = true
	}
	for i := range data.Installs {
		if data.Installs[i].kind() == "skill" && data.Installs[i].Skill != "" {
			known[data.Installs[i].Skill] = true
		}
	}
	var missing []string
	for _, name := range names {
		if !known[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		die("unknown skill: %s", strings.Join(missing, ", "))
	}
}

func resolveRepoNames(skills []Skill, names []string) {
	known := skillsByName(skills)
	var missing []string
	for _, name := range names {
		if _, ok := known[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		die("unknown skill: %s", strings.Join(missing, ", "))
	}
}

// --- commands ---------------------------------------------------------------

func skillCmdInstall(args *cliArgs, cat *catalog, names []string, data *stateData, allowEmpty bool) {
	scopes := selectedScopes(args, "global")
	platformFilter := selectedPlatforms(args.platforms)
	var project string
	if contains(scopes, "local") {
		project = projectDir(args)
	}
	if args.directory != "" && !contains(scopes, "local") {
		die("--directory is used with --local")
	}
	byName := skillsByName(cat.skills)
	type choice struct {
		skill     Skill
		platforms []string
	}
	var chosen []choice
	if args.all {
		for _, skill := range cat.skills {
			if platforms := platformsForSkill(skill, platformFilter, false); len(platforms) > 0 {
				chosen = append(chosen, choice{skill, platforms})
			}
		}
		if len(chosen) == 0 {
			if allowEmpty {
				return
			}
			die("no tools match the selected platforms")
		}
	} else {
		resolveRequested(cat.skills, names, data)
		for _, name := range names {
			skill, ok := byName[name]
			if !ok {
				die("unknown skill: %s", name)
			}
			chosen = append(chosen, choice{skill, platformsForSkill(skill, platformFilter, true)})
		}
	}

	var plans []*Plan
	skipKeys := map[entryKey]bool{}
	var targets []struct {
		skill  Skill
		target Target
	}
	for _, c := range chosen {
		for _, scope := range scopes {
			for _, platform := range c.platforms {
				target := makeTarget(platform, scope, project, c.skill.Name)
				targets = append(targets, struct {
					skill  Skill
					target Target
				}{c.skill, target})
				skipKeys[target.key(c.skill.Name)] = true
			}
		}
	}
	for _, t := range targets {
		plans = append(plans, planInstall(t.skill, t.target, data, skipKeys, false, args.force))
	}

	if args.dryRun {
		fmt.Println("dry-run: no files will be changed")
	}
	for _, plan := range plans {
		verb := "installed"
		if args.dryRun {
			verb = "would install"
		}
		printPlan(fmt.Sprintf("%s %s → %s (%s)", verb, plan.Skill.Name, plan.Target.Platform, plan.Target.Scope), plan)
		applyPlan(plan, args.dryRun)
		if !args.dryRun {
			recordSkillInstall(data, plan)
			saveState(data)
		}
	}
}

func skillCmdUninstall(args *cliArgs, cat *catalog, names []string, data *stateData, allowEmpty bool) {
	scopes := selectedScopes(args, "global")
	platformFilter := selectedPlatforms(args.platforms)
	var project string
	if contains(scopes, "local") {
		project = projectDir(args)
	}
	if args.directory != "" && !contains(scopes, "local") {
		die("--directory is used with --local")
	}
	byName := skillsByName(cat.skills)

	type selection struct {
		name     string
		target   Target
		manifest *manifest
	}
	var selected []selection
	if args.all {
		for i := range data.Installs {
			e := &data.Installs[i]
			if e.kind() != "skill" || !contains(scopes, e.Scope) {
				continue
			}
			if platformFilter != nil && !contains(platformFilter, e.Platform) {
				continue
			}
			if e.Scope == "local" && e.Project != project {
				continue
			}
			var m *manifest
			if skill, ok := byName[e.Skill]; ok {
				loaded := skill.Manifest
				m = &loaded
			}
			target := makeTarget(e.Platform, e.Scope, map[bool]string{true: project, false: ""}[e.Scope == "local"], e.Skill)
			if resolve(e.SkillDir) != resolve(target.SkillDir) {
				fmt.Printf("skipping %s → %s (%s): recorded at %s\n", e.Skill, e.Platform, e.Scope, e.SkillDir)
				continue
			}
			selected = append(selected, selection{e.Skill, target, m})
		}
	} else {
		if !args.force {
			resolveRequested(cat.skills, names, data)
		}
		for _, name := range names {
			skill, inRepo := byName[name]
			platforms := platformOrder
			if inRepo {
				platforms = platformsForSkill(skill, platformFilter, true)
			} else if platformFilter != nil {
				platforms = platformFilter
			}
			var m *manifest
			if inRepo {
				loaded := skill.Manifest
				m = &loaded
			}
			matched := false
			for _, scope := range scopes {
				for _, platform := range platforms {
					target := makeTarget(platform, scope, project, name)
					if findEntry(data.Installs, target.key(name), "skill") != nil {
						selected = append(selected, selection{name, target, m})
						matched = true
					} else if args.force && present(target.SkillDir) {
						selected = append(selected, selection{name, target, m})
						matched = true
					}
				}
			}
			if !matched {
				fmt.Printf("%s is not installed for %s (%s)\n", name, strings.Join(platforms, ", "), strings.Join(scopes, " and "))
			}
		}
	}

	if args.all && len(selected) == 0 {
		if !allowEmpty {
			fmt.Println("nothing installed for the selected scope")
		}
		return
	}

	skipKeys := map[entryKey]bool{}
	for _, s := range selected {
		skipKeys[s.target.key(s.name)] = true
	}
	var plans []*Plan
	for _, s := range selected {
		if plan := planUninstall(s.name, s.target, s.manifest, data, skipKeys, args.force); plan != nil {
			plans = append(plans, plan)
		}
	}

	if args.dryRun && len(plans) > 0 {
		fmt.Println("dry-run: no files will be changed")
	}
	for _, plan := range plans {
		verb := "uninstalled"
		if args.dryRun {
			verb = "would uninstall"
		}
		printPlan(fmt.Sprintf("%s %s → %s (%s)", verb, plan.Skill.Name, plan.Target.Platform, plan.Target.Scope), plan)
		applyPlan(plan, args.dryRun)
		if !args.dryRun {
			dropInstall(data, plan.Skill.Name, plan.Target, "skill")
			saveState(data)
		}
	}
}

func skillCmdUpdate(args *cliArgs, cat *catalog, names []string, data *stateData, allowEmpty bool) bool {
	scopes := selectedScopes(args, "global")
	platformFilter := selectedPlatforms(args.platforms)
	var project string
	if contains(scopes, "local") {
		project = projectDir(args)
	}
	if args.directory != "" && !contains(scopes, "local") {
		die("--directory is used with --local")
	}
	byName := skillsByName(cat.skills)
	var chosen []Skill
	if len(names) > 0 {
		resolveRepoNames(cat.skills, names)
		for _, name := range names {
			chosen = append(chosen, byName[name])
		}
	} else {
		chosen = cat.skills
	}

	var outdated, current []struct {
		skill  Skill
		target Target
	}
	installedNames := map[string]bool{}
	for _, skill := range chosen {
		platforms := platformsForSkill(skill, platformFilter, len(names) > 0)
		if len(platforms) == 0 {
			continue
		}
		for _, scope := range scopes {
			for _, platform := range platforms {
				target := makeTarget(platform, scope, project, skill.Name)
				if !isFile(filepath.Join(target.SkillDir, "SKILL.md")) {
					continue
				}
				installedNames[skill.Name] = true
				if repoIsNewer(skill, target) {
					outdated = append(outdated, struct {
						skill  Skill
						target Target
					}{skill, target})
				} else if len(names) > 0 {
					current = append(current, struct {
						skill  Skill
						target Target
					}{skill, target})
				}
			}
		}
	}

	if len(names) > 0 {
		for _, name := range names {
			if !installedNames[name] {
				fmt.Printf("%s is not installed\n", name)
			}
		}
		for _, c := range current {
			fmt.Printf("%s is up to date for %s (%s)\n", c.skill.Name, c.target.Platform, c.target.Scope)
		}
	}
	if len(outdated) == 0 {
		if len(names) == 0 && !allowEmpty {
			fmt.Println("nothing to update")
		}
		return false
	}

	if args.dryRun {
		fmt.Println("dry-run: no files will be changed")
	}
	for _, o := range outdated {
		skill, target := o.skill, o.target
		m := skill.Manifest
		key := target.key(skill.Name)
		verb := "updated"
		if args.dryRun {
			verb = "would update"
		}
		fmt.Printf("%s %s → %s (%s)\n", verb, skill.Name, target.Platform, target.Scope)
		entry := findEntry(data.Installs, key, "skill")
		if entry != nil {
			removal := planUninstall(skill.Name, target, &m, data, map[entryKey]bool{key: true}, false)
			if removal != nil {
				for _, action := range removal.Actions {
					if action.Kind == "remove" {
						fmt.Printf("  remove %s\n", display(action.Path))
					} else if action.Kind == "keep" {
						fmt.Printf("  keep %s (%s)\n", display(action.Path), action.Note)
					}
				}
				if !args.dryRun {
					applyPlan(removal, false)
					dropInstall(data, skill.Name, target, "skill")
					saveState(data)
				}
			}
		} else if present(target.SkillDir) {
			if isSymlink(target.SkillDir) {
				die("%s is a symlink; refusing to update over it. Remove the link manually.", displayLink(target.SkillDir))
			}
			fmt.Printf("  remove %s\n", display(target.SkillDir))
			if !args.dryRun {
				trashPath(target.SkillDir)
			}
		}
		fresh := planInstall(skill, target, data, map[entryKey]bool{key: true}, true, args.force)
		installVerb := "installed"
		if args.dryRun {
			installVerb = "would install"
		}
		printPlan(fmt.Sprintf("%s %s → %s (%s)", installVerb, skill.Name, target.Platform, target.Scope), fresh)
		if !args.dryRun {
			applyPlan(fresh, false)
			recordSkillInstall(data, fresh)
			saveState(data)
		}
	}
	return true
}

func skillInstallStatus(skill Skill, data *stateData) [][3]string {
	project := resolve(cwd())
	var found [][3]string
	for _, scope := range []string{"global", "local"} {
		for _, platform := range skill.Platforms {
			p := ""
			if scope == "local" {
				p = project
			}
			target := makeTarget(platform, scope, p, skill.Name)
			if !isFile(filepath.Join(target.SkillDir, "SKILL.md")) {
				continue
			}
			entry := findEntry(data.Installs, target.key(skill.Name), "skill")
			mark := "◎"
			if entry != nil {
				mark = "◉"
				if repoIsNewer(skill, target) {
					mark = "▲"
				}
			}
			found = append(found, [3]string{scope, platform, mark})
		}
	}
	return found
}

func skillCmdAbout(args *cliArgs, cat *catalog, name string, data *stateData) {
	skill := skillsByName(cat.skills)[name]
	meta := parseFrontmatter(filepath.Join(skill.Path, "SKILL.md"))
	version := formatVersion(repoMtime(skill))
	title := skill.Name
	if meta["name"] != "" {
		title = meta["name"]
	}
	fields := [][2]string{
		{"version", version},
		{"platforms", strings.Join(skill.Platforms, ", ")},
		{"path", relIn(skill.Root, skill.Path)},
	}
	for _, key := range sortedKeys(meta) {
		if key != "name" && key != "description" && meta[key] != "" {
			fields = append(fields, [2]string{key, meta[key]})
		}
	}
	printAbout(title, "skill", fields, skillInstallStatus(skill, data), strings.TrimSpace(meta["description"]))
}

func scanSkillNames(skillsDir string) []string {
	if !isDir(skillsDir) {
		return nil
	}
	var names []string
	entries, _ := os.ReadDir(skillsDir)
	for _, child := range entries {
		if child.IsDir() && !isSymlink(filepath.Join(skillsDir, child.Name())) &&
			isFile(filepath.Join(skillsDir, child.Name(), "SKILL.md")) {
			names = append(names, child.Name())
		}
	}
	return names
}

func collectInstalledSkills(args *cliArgs, cat *catalog, names []string, data *stateData, scopes []string, project string) []installedGroup {
	nameFilter := map[string]bool{}
	for _, n := range names {
		nameFilter[n] = true
	}
	repoNames := map[string]bool{}
	repoSkills := skillsByName(cat.skills)
	for _, s := range cat.skills {
		repoNames[s.Name] = true
	}
	platforms := selectedPlatforms(args.platforms)
	if platforms == nil {
		platforms = platformOrder
	}
	var groups []installedGroup
	for _, scope := range scopes {
		for _, platform := range platforms {
			scopeProject := ""
			if scope == "local" {
				scopeProject = project
			}
			skillsDir, _ := platformPaths(platform, scope, scopeProject)
			group := installedGroup{Scope: scope, Platform: platform, Location: skillsDir}
			groups = append(groups, group)
			onDisk := scanSkillNames(skillsDir)
			var recorded []installEntry
			for i := range data.Installs {
				e := &data.Installs[i]
				if e.kind() != "skill" || e.Platform != platform || e.Scope != scope {
					continue
				}
				if scope == "local" && e.Project != resolve(project) && e.Project != project {
					continue
				}
				if filepath.Dir(resolve(e.SkillDir)) != resolve(skillsDir) {
					continue
				}
				recorded = append(recorded, *e)
			}
			nameSet := map[string]bool{}
			for _, n := range onDisk {
				nameSet[n] = true
			}
			for _, e := range recorded {
				nameSet[e.Skill] = true
			}
			var names2 []string
			for n := range nameSet {
				names2 = append(names2, n)
			}
			sort.Strings(names2)
			if len(nameFilter) > 0 {
				var filtered []string
				for _, n := range names2 {
					if nameFilter[n] {
						filtered = append(filtered, n)
					}
				}
				names2 = filtered
			}
			if len(names2) == 0 {
				continue
			}
			recordedByName := map[string]installEntry{}
			for _, e := range recorded {
				recordedByName[e.Skill] = e
			}
			onDiskSet := map[string]bool{}
			for _, n := range onDisk {
				onDiskSet[n] = true
			}
			for _, name := range names2 {
				entry, hasEntry := recordedByName[name]
				repoSkill, inRepo := repoSkills[name]
				target := makeTarget(platform, scope, scopeProject, name)
				newer := hasEntry && inRepo && repoIsNewer(repoSkill, target)
				var notes []string
				if hasEntry {
					if !onDiskSet[name] {
						notes = append(notes, "not on disk")
					}
					if len(entry.ExternalFiles) > 0 {
						parents := map[string]int{}
						for _, item := range entry.ExternalFiles {
							parents[filepath.Dir(item)]++
						}
						var bits []string
						for _, parent := range sortedKeys(parents) {
							label := "file"
							if parents[parent] != 1 {
								label = "files"
							}
							bits = append(bits, fmt.Sprintf("%d %s in %s", parents[parent], label, display(parent)))
						}
						notes = append(notes, "+ "+strings.Join(bits, ", "))
					}
				}
				mark := "◎"
				if hasEntry {
					mark = "◉"
					if newer {
						mark = "▲"
					}
				}
				version := ""
				if args.showVersion {
					var mtime time.Time
					var ok bool
					if inRepo && onDiskSet[name] {
						mtime, ok = installedContentMtime(repoSkill, target)
					} else if onDiskSet[name] {
						mtime, ok = directoryMtime(target.SkillDir)
					}
					if ok {
						version = " - " + formatVersion(mtime)
					}
					if newer && inRepo {
						repoVersion := formatVersion(repoMtime(repoSkill))
						if version != "" {
							version = version + " < " + repoVersion
						} else {
							version = " - " + repoVersion
						}
					}
				}
				suffix := ""
				if len(notes) > 0 {
					suffix = "  (" + strings.Join(notes, ", ") + ")"
				}
				group.Rows = append(group.Rows, fmt.Sprintf("%s %s%s%s", mark, name, version, suffix))
			}
			groups[len(groups)-1] = group
		}
	}
	return groups
}
