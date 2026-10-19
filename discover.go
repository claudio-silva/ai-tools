package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// manifestName is the optional metadata file: in a skill directory it
// describes that skill, in any other directory an MCP server, and at the
// repository root it can describe any tool by name.
const manifestName = "aitools.json"

// skillContainers are searched for skills, as `npx skills` does: a skill is a
// directory holding SKILL.md, up to containerDepth levels below a container.
var skillContainers = []string{
	"skills", "skills/.curated", "skills/.experimental", "skills/.system",
	".agents/skills", ".claude/skills",
}

const containerDepth = 3

var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// walkSkipDirs are never searched for tools.
var walkSkipDirs = map[string]bool{"node_modules": true, "vendor": true}

type repoManifest struct {
	path    string
	exclude []string // applied to every skill
	skills  map[string]map[string]any
	mcps    map[string]map[string]any
}

// discover finds every skill and MCP server in a repository.
func discover(root, srcURL string) ([]Skill, []*McpServer) {
	repo := loadRepoManifest(root)
	skillDirs := findSkillDirs(root)

	var skills []Skill
	names := map[string]string{}
	isSkillDir := map[string]bool{}
	for _, dir := range skillDirs {
		isSkillDir[dir] = true
		skillMD := filepath.Join(dir, "SKILL.md")
		meta := parseFrontmatter(skillMD)
		name := meta["name"]
		if name == "" || meta["description"] == "" {
			warn("%s: frontmatter needs name and description; skipped", relIn(root, skillMD))
			continue
		}
		if !skillNameRe.MatchString(name) {
			warn("%s: name %q must be lowercase letters, digits, and hyphens; skipped", relIn(root, skillMD), name)
			continue
		}
		if problem := frontmatterProblem(skillMD); problem != "" {
			warn("%s: %s; other installers (e.g. npx skills) will skip this skill", relIn(root, skillMD), problem)
		}
		if prev, dup := names[name]; dup {
			die("duplicate skill name %s: %s and %s", name, relIn(root, prev), relIn(root, dir))
		}
		names[name] = dir

		m := manifest{}
		local := filepath.Join(dir, manifestName)
		fromRepo, inRepo := repo.skills[name]
		switch {
		case dir != root && isFile(local) && inRepo:
			die("%s is described twice: %s and %s", name, relIn(root, local), relIn(root, repo.path))
		case dir != root && isFile(local):
			m = parseSkillManifest(readJSONObject(root, local), relIn(root, local))
		case inRepo:
			m = parseSkillManifest(fromRepo, relIn(root, repo.path)+" skills."+name)
		}
		m.exclude = append(append([]string{}, repo.exclude...), m.exclude...)
		platforms := m.platforms
		if platforms == nil {
			platforms = platformOrder
		}
		for _, entry := range m.install {
			for _, p := range entry.platforms {
				if !contains(platforms, p) {
					die("%s: files entry %s targets %s, which the skill does not support", name, entry.from, p)
				}
			}
		}
		skills = append(skills, Skill{
			Name: name, Path: dir, Platforms: platforms, Manifest: m,
			Root: root, Source: srcURL,
		})
	}
	for name := range repo.skills {
		if _, ok := names[name]; !ok {
			die("%s: skills.%s matches no SKILL.md in the repository", relIn(root, repo.path), name)
		}
	}

	var mcps []*McpServer
	mcpNames := map[string]string{}
	addMcp := func(server *McpServer, where string) {
		if prev, dup := mcpNames[server.Name]; dup {
			die("duplicate MCP name %s: %s and %s", server.Name, prev, where)
		}
		mcpNames[server.Name] = where
		mcps = append(mcps, server)
	}
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if p != root && skipWalkDir(d.Name()) || isSkillDir[p] || isFile(filepath.Join(p, "SKILL.md")) {
			return filepath.SkipDir
		}
		manifestPath := filepath.Join(p, manifestName)
		if p == root || !isFile(manifestPath) {
			return nil
		}
		rel := relIn(root, manifestPath)
		if _, dup := repo.mcps[d.Name()]; dup {
			die("%s is described twice: %s and %s", d.Name(), rel, relIn(root, repo.path))
		}
		addMcp(parseMcpManifest(readJSONObject(root, manifestPath), rel, d.Name(), p, manifestPath, root, srcURL), rel)
		return nil
	})
	for _, name := range sortedKeys(repo.mcps) {
		data := repo.mcps[name]
		rel := relIn(root, repo.path) + " mcp." + name
		dir := root
		if v, ok := data["path"]; ok {
			s, isStr := v.(string)
			if !isStr || s == "" || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") || contains(strings.Split(s, "/"), "..") {
				die("%s: path must be a directory inside the repository", rel)
			}
			dir = filepath.Join(root, filepath.FromSlash(s))
			if !isDir(dir) {
				die("%s: path %s is not a directory", rel, s)
			}
			delete(data, "path")
		}
		addMcp(parseMcpManifest(data, rel, name, dir, repo.path, root, srcURL), rel)
	}
	return skills, mcps
}

func skipWalkDir(name string) bool {
	return strings.HasPrefix(name, ".") || skipDirNames[name] || walkSkipDirs[name]
}

// findSkillDirs mirrors `npx skills` discovery: a SKILL.md at the root makes
// the repository a single skill; otherwise the containers are searched, and
// when they hold nothing, the whole tree. A skill directory shadows any
// SKILL.md nested below it.
func findSkillDirs(root string) []string {
	if isFile(filepath.Join(root, "SKILL.md")) {
		return []string{root}
	}
	seen := map[string]bool{}
	var found []string
	for _, c := range skillContainers {
		walkSkillDirs(filepath.Join(root, filepath.FromSlash(c)), containerDepth, seen, &found)
	}
	if len(found) == 0 {
		walkSkillDirs(root, -1, seen, &found)
	}
	return found
}

func walkSkillDirs(dir string, depth int, seen map[string]bool, found *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || skipWalkDir(e.Name()) {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		if isFile(filepath.Join(sub, "SKILL.md")) {
			if !seen[sub] {
				seen[sub] = true
				*found = append(*found, sub)
			}
			continue
		}
		if depth != 1 {
			walkSkillDirs(sub, depth-1, seen, found)
		}
	}
}

// frontmatterProblem flags plain YAML values that strict parsers reject, such
// as an unquoted description containing ": ".
func frontmatterProblem(skillMD string) string {
	text := readFile(skillMD)
	if !strings.HasPrefix(text, "---\n") {
		return "SKILL.md has no frontmatter"
	}
	end := strings.Index(text[3:], "\n---")
	if end == -1 {
		return "SKILL.md frontmatter is not closed"
	}
	for _, line := range strings.Split(text[4:3+end], "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' || line[0] == '-' {
			continue
		}
		idx := strings.Index(line, ":")
		if idx == -1 {
			continue
		}
		value := strings.TrimSpace(line[idx+1:])
		if value == "" || strings.ContainsRune(`"'|>[{`, rune(value[0])) {
			continue
		}
		if strings.Contains(value, ": ") || strings.HasSuffix(value, ":") {
			return fmt.Sprintf("frontmatter %s is not valid YAML (quote the value; it contains \": \")", strings.TrimSpace(line[:idx]))
		}
	}
	return ""
}

func readJSONObject(root, file string) map[string]any {
	raw, err := os.ReadFile(file)
	if err != nil {
		die("%s: %v", relIn(root, file), err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		die("%s: %v", relIn(root, file), err)
	}
	return data
}

func loadRepoManifest(root string) repoManifest {
	repo := repoManifest{path: filepath.Join(root, manifestName)}
	if !isFile(repo.path) {
		return repo
	}
	data := readJSONObject(root, repo.path)
	rel := relIn(root, repo.path)
	table := func(key string) map[string]map[string]any {
		raw, ok := data[key]
		if !ok {
			return nil
		}
		obj, ok := raw.(map[string]any)
		if !ok {
			die("%s: %s must be an object keyed by tool name", rel, key)
		}
		out := map[string]map[string]any{}
		for name, v := range obj {
			entry, ok := v.(map[string]any)
			if !ok {
				die("%s: %s.%s must be an object", rel, key, name)
			}
			out[name] = entry
		}
		return out
	}
	for key := range data {
		if key != "skills" && key != "mcp" && key != "exclude" {
			die("%s: unknown key %s (expected skills, mcp, exclude)", rel, key)
		}
	}
	repo.exclude = parseExclude(data["exclude"], rel)
	repo.skills = table("skills")
	repo.mcps = table("mcp")
	for name := range repo.mcps {
		if !skillNameRe.MatchString(name) {
			die("%s: MCP name %q must be lowercase letters, digits, and hyphens", rel, name)
		}
	}
	return repo
}

func checkKeys(data map[string]any, rel string, allowed ...string) {
	for _, key := range sortedKeys(data) {
		if !contains(allowed, key) {
			die("%s: unknown key %s (expected %s)", rel, key, strings.Join(allowed, ", "))
		}
	}
}

// parsePlatforms returns nil when the key is absent: every platform.
func parsePlatforms(v any, rel string) []string {
	if v == nil {
		return nil
	}
	items, ok := v.([]any)
	if !ok || len(items) == 0 {
		die("%s: platforms must be a non-empty list of names", rel)
	}
	var out []string
	for _, item := range items {
		name, ok := item.(string)
		if !ok || !contains(platformOrder, name) {
			die("%s: unknown platform %v (expected %s)", rel, item, strings.Join(platformOrder, ", "))
		}
		out = append(out, name)
	}
	var ordered []string
	for _, p := range platformOrder {
		if contains(out, p) {
			ordered = append(ordered, p)
		}
	}
	return ordered
}

func parseStrings(v any, rel, key string) []string {
	var out []string
	for _, item := range toSlice(v, rel, key) {
		s, ok := item.(string)
		if !ok || s == "" {
			die("%s: %s must be a list of paths", rel, key)
		}
		out = append(out, s)
	}
	return out
}

// parseEntries reads a files list. Skills take only {from, to?, platforms?}
// objects (everything else is copied by default); MCP servers also take plain
// path strings.
func parseEntries(v any, rel string, skill bool) []manifestEntry {
	var out []manifestEntry
	for _, item := range toSlice(v, rel, "files") {
		switch e := item.(type) {
		case string:
			if skill {
				die("%s: skill files entries must be {\"from\", \"to\"} objects; the skill directory is copied by default", rel)
			}
			if e == "" {
				die("%s: empty files path", rel)
			}
			out = append(out, manifestEntry{from: e})
		case map[string]any:
			allowed := []string{"from", "to"}
			if skill {
				allowed = append(allowed, "platforms")
			}
			checkKeys(e, rel+" files", allowed...)
			from, _ := e["from"].(string)
			to, toOK := e["to"].(string)
			if from == "" {
				die("%s: files objects need a string from", rel)
			}
			if _, has := e["to"]; has && (!toOK || to == "") {
				die("%s: files to must be a non-empty string", rel)
			}
			if !skill && to == "" {
				die("%s: files objects need string from and to", rel)
			}
			out = append(out, manifestEntry{from: from, to: to, platforms: parsePlatforms(e["platforms"], rel+" files "+from)})
		default:
			die("%s: files entries must be strings or objects", rel)
		}
	}
	return out
}

func parseSkillManifest(data map[string]any, rel string) manifest {
	checkKeys(data, rel, "platforms", "exclude", "files", "remove")
	m := manifest{
		platforms: parsePlatforms(data["platforms"], rel),
		exclude:   parseExclude(data["exclude"], rel),
		install:   parseEntries(data["files"], rel, true),
		remove:    parseStrings(data["remove"], rel, "remove"),
	}
	return m
}

// parseExclude reads exclude patterns: paths or globs relative to a skill
// directory; a match on a directory excludes everything below it.
func parseExclude(v any, rel string) []string {
	patterns := parseStrings(v, rel, "exclude")
	for _, pattern := range patterns {
		p := strings.TrimSuffix(pattern, "/")
		if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || contains(strings.Split(p, "/"), "..") {
			die("%s: exclude must stay inside the skill directory (%s)", rel, pattern)
		}
		if _, err := path.Match(p, ""); err != nil {
			die("%s: bad exclude pattern %s", rel, pattern)
		}
	}
	return patterns
}

func parseMcpManifest(data map[string]any, rel, name, dir, manifestFile, root, srcURL string) *McpServer {
	checkKeys(data, rel, "platforms", "settings", "files", "message")
	if !skillNameRe.MatchString(name) {
		die("%s: MCP name %q must be lowercase letters, digits, and hyphens", rel, name)
	}
	platforms := parsePlatforms(data["platforms"], rel)
	if platforms == nil {
		platforms = platformOrder
	}
	settings, ok := data["settings"].(map[string]any)
	if !ok || len(settings) == 0 {
		die("%s: settings must be a non-empty object", rel)
	}
	message := ""
	if v, has := data["message"]; has {
		s, ok := v.(string)
		if !ok {
			die("%s: message must be a string", rel)
		}
		message = strings.TrimSpace(s)
	}
	binary := filepath.Join(dir, "bin", name)
	hasBinary := isFile(binary)
	if !hasBinary {
		binary = ""
	}
	files := parseEntries(data["files"], rel, false)
	for _, ph := range collectPlaceholders(settings) {
		if ph == "BINARY" && !hasBinary {
			die("%s: settings use $BINARY but %s is missing", rel, relIn(root, filepath.Join(dir, "bin", name)))
		}
		if ph == "SERVER_DIR" && len(files) == 0 {
			die("%s: settings use $SERVER_DIR but no files are shipped", rel)
		}
	}
	return &McpServer{
		Name:         name,
		Path:         dir,
		Platforms:    platforms,
		Settings:     settings,
		Message:      message,
		Binary:       binary,
		Files:        files,
		ManifestFile: manifestFile,
		Root:         root,
		Source:       srcURL,
	}
}
