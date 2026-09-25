package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// --- helpers ----------------------------------------------------------------

func mustDie(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected die(), got success")
		}
		if _, ok := r.(*userError); !ok {
			t.Fatalf("expected userError panic, got %v", r)
		}
	}()
	fn()
}

func sortStrings(s []string) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixtureRepo(t *testing.T, skillDir string, manifestJSON string, files map[string]string) (root string, skill Skill) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, skillDir, "manifest.json"), manifestJSON)
	writeFile(t, filepath.Join(root, skillDir, "SKILL.md"), "---\nname: test\ndescription: d\n---\nbody\n")
	for rel, content := range files {
		writeFile(t, filepath.Join(root, skillDir, rel), content)
	}
	skill = Skill{
		Name:   "test",
		Folder: "Shared",
		Path:   filepath.Join(root, skillDir),
		Root:   root,
		Source: root,
	}
	return root, skill
}

func testTarget(t *testing.T, project string) Target {
	t.Helper()
	base := t.TempDir()
	return Target{
		Platform:     "cursor",
		Scope:        "local",
		Project:      project,
		SkillsDir:    filepath.Join(base, "skills"),
		PlatformHome: filepath.Join(base, ".cursor"),
		SkillDir:     filepath.Join(base, "skills", "test"),
	}
}

// --- source parsing ----------------------------------------------------------

func TestLooksLikeSource(t *testing.T) {
	yes := []string{
		"@gh/o/r", "@bb/o/r", "@gl/o/r",
		"https://github.com/o/r", "http://x/o/r", "ssh://git@github.com/o/r",
		"git@github.com:o/r", "git@gitlab.com:o/r.git",
		"/abs/path", "~/rel", "./rel", "../rel", ".", "..",
		"owner/repo", "owner/group/repo",
	}
	for _, arg := range yes {
		if !looksLikeSource(arg, false) {
			t.Errorf("looksLikeSource(%q) = false, want true", arg)
		}
	}
	no := []string{"imagen", "cua-driver", "my_skill", ""}
	for _, arg := range no {
		if looksLikeSource(arg, false) {
			t.Errorf("looksLikeSource(%q) = true, want false", arg)
		}
	}
	// bare dir counts only when bareDir is set
	dir := t.TempDir()
	old, _ := os.Getwd()
	defer os.Chdir(old)
	os.Chdir(dir)
	os.Mkdir("somedir", 0o755)
	if looksLikeSource("somedir", false) {
		t.Error("bare dir accepted without bareDir")
	}
	if !looksLikeSource("somedir", true) {
		t.Error("bare dir rejected with bareDir")
	}
}

func TestParseSource(t *testing.T) {
	cases := []struct{ arg, kind, host, repo, url string }{
		{"@gh/o/r", "remote", "github", "o/r", "https://github.com/o/r"},
		{"@bb/o/r", "remote", "bitbucket", "o/r", "https://bitbucket.org/o/r"},
		{"@gl/g/sub/r", "remote", "gitlab", "g/sub/r", "https://gitlab.com/g/sub/r"},
		{"o/r", "remote", "github", "o/r", "https://github.com/o/r"},
		{"https://github.com/o/r", "remote", "github", "o/r", "https://github.com/o/r"},
		{"https://github.com/o/r.git", "remote", "github", "o/r", "https://github.com/o/r"},
		{"git@github.com:o/r.git", "remote", "github", "o/r", "https://github.com/o/r"},
		{"ssh://git@gitlab.com/g/r.git", "remote", "gitlab", "g/r", "https://gitlab.com/g/r"},
	}
	for _, c := range cases {
		s := parseSource(c.arg)
		if s.Kind != c.kind || s.Host != c.host || s.RepoPath != c.repo || s.URL != c.url {
			t.Errorf("parseSource(%q) = {%s %s %s %s}", c.arg, s.Kind, s.Host, s.RepoPath, s.URL)
		}
	}
}

func TestParseSourceLocal(t *testing.T) {
	dir := t.TempDir()
	s := parseSource(dir)
	if s.Kind != "local" || s.LocalPath != resolve(dir) || s.URL != resolve(dir) {
		t.Errorf("local parse: %+v", s)
	}
	s = parseSource(".")
	if s.Kind != "local" || !filepath.IsAbs(s.LocalPath) {
		t.Errorf("'.' parse: %+v", s)
	}
}

func TestParseSourceRejects(t *testing.T) {
	mustDie(t, func() { parseSource("notasource") })
	mustDie(t, func() { parseSource("https://example.com/o/r") })
	mustDie(t, func() { parseSource("https://github.com/onlyowner") })
	mustDie(t, func() { parseSource("https://github.com/a/b/c") }) // too deep for github
	mustDie(t, func() { parseSource("nosuchdir-xyz") })
}

// --- arg parsing -------------------------------------------------------------

func TestParseArgsRepoSplit(t *testing.T) {
	a := parseArgs([]string{"install", "@gh/o/r", "tool1", "tool2", "-p", "codex", "-n"})
	if a.repo != "@gh/o/r" {
		t.Errorf("repo = %q", a.repo)
	}
	if !reflect.DeepEqual(a.names, []string{"tool1", "tool2"}) {
		t.Errorf("names = %v", a.names)
	}
	if a.platforms[0] != "codex" || !a.dryRun {
		t.Errorf("flags wrong: %+v", a)
	}
}

func TestParseArgsNoRepo(t *testing.T) {
	a := parseArgs([]string{"install", "tool1", "-g"})
	if a.repo != "" || !reflect.DeepEqual(a.names, []string{"tool1"}) || !a.scopeGlobal {
		t.Errorf("%+v", a)
	}
}

func TestParseArgsBundles(t *testing.T) {
	a := parseArgs([]string{"install", "x", "-gn"})
	if !a.scopeGlobal || !a.dryRun {
		t.Errorf("bundle -gn: %+v", a)
	}
	a = parseArgs([]string{"install", "x", "--global", "--dry-run", "--platform=codex"})
	if !a.scopeGlobal || !a.dryRun || a.platforms[0] != "codex" {
		t.Errorf("long flags: %+v", a)
	}
	a = parseArgs([]string{"install", "x", "-C/tmp/proj", "-p", "cursor,codex"})
	if a.directory != "/tmp/proj" || len(a.platforms) != 1 || a.platforms[0] != "cursor,codex" {
		t.Errorf("-C/-p: %+v", a)
	}
}

func TestParseArgsDashDash(t *testing.T) {
	a := parseArgs([]string{"install", "x", "--", "-weird"})
	if len(a.names) != 2 || a.names[1] != "-weird" {
		t.Errorf("-- handling: %v", a.names)
	}
}

// --- manifest expansion --------------------------------------------------------

func TestExpandInstallBasic(t *testing.T) {
	root, skill := fixtureRepo(t, "skills/Shared/test",
		`{"install": ["SKILL.md", "agents", "extra.txt"]}`,
		map[string]string{
			"agents/a.md":      "a",
			"agents/sub/b.md":  "b",
			"extra.txt":        "x",
			"notinstalled.txt": "n",
			"agents/.DS_Store": "junk",
		})
	target := testTarget(t, t.TempDir())
	m := loadManifest(skill)
	pairs := expandInstall(skill, m, target)
	var dests []string
	for _, p := range pairs {
		rel, _ := filepath.Rel(resolve(target.SkillDir), p.dest)
		if rel != "" && !strings.HasPrefix(rel, "..") {
			dests = append(dests, rel)
		}
	}
	sortStrings(dests)
	got := strings.Join(dests, ",")
	want := "SKILL.md,agents/a.md,agents/sub/b.md,extra.txt"
	if got != want {
		t.Errorf("dests = %s, want %s", got, want)
	}
	_ = root
}

func TestExpandInstallFromTo(t *testing.T) {
	target := testTarget(t, t.TempDir())
	_, skill := fixtureRepo(t, "skills/Shared/test",
		`{"install": ["SKILL.md", {"from": "te-agents/*.toml", "to": "$PLATFORM_HOME/agents/"}]}`,
		map[string]string{"te-agents/a.toml": "a", "te-agents/b.toml": "b"})
	pairs := expandInstall(skill, loadManifest(skill), target)
	destDir := filepath.Join(target.PlatformHome, "agents")
	found := 0
	for _, p := range pairs {
		if strings.HasSuffix(p.src, ".toml") {
			found++
			if filepath.Dir(p.dest) != resolve(destDir) {
				t.Errorf("external dest = %s, want under %s", p.dest, destDir)
			}
		}
	}
	if found != 2 {
		t.Errorf("matched %d toml files, want 2", found)
	}
}

func TestExpandInstallRequiresSkillMD(t *testing.T) {
	target := testTarget(t, t.TempDir())
	_, skill := fixtureRepo(t, "skills/Shared/test",
		`{"install": ["other.txt"]}`,
		map[string]string{"other.txt": "x"})
	mustDie(t, func() { expandInstall(skill, loadManifest(skill), target) })
}

func TestExpandVars(t *testing.T) {
	target := testTarget(t, t.TempDir())
	vars := variablesFor(target)
	got := expandVars("$SKILLS_DIR/x", vars, "test")
	if got != filepath.Join(target.SkillsDir, "x") {
		t.Errorf("expandVars = %s", got)
	}
	got = expandVars("$SKILL_DIR/f", vars, "test")
	if got != filepath.Join(target.SkillDir, "f") {
		t.Errorf("expandVars SKILL_DIR = %s", got)
	}
	mustDie(t, func() { expandVars("$NOPE/x", vars, "test") })
	mustDie(t, func() { expandVars("relative/path", vars, "test") })
}

// --- mcp settings --------------------------------------------------------------

func TestCollectPlaceholders(t *testing.T) {
	v := map[string]any{
		"command": "$BINARY",
		"args":    []any{"$SERVER_DIR/x.py"},
		"env":     map[string]any{"KEY": "$MY_KEY", "OTHER": "plain"},
	}
	got := collectPlaceholders(v)
	want := []string{"BINARY", "MY_KEY", "SERVER_DIR"}
	if len(got) != len(want) {
		t.Fatalf("placeholders = %v", got)
	}
	for _, w := range want {
		if !contains(got, w) {
			t.Errorf("missing %s in %v", w, got)
		}
	}
}

func TestResolveSettingsFill(t *testing.T) {
	server := &McpServer{
		Name: "test",
		Settings: map[string]any{
			"command": "$BINARY",
			"env":     map[string]any{"K": "$TESTKEY_XYZ"},
		},
	}
	t.Setenv("TESTKEY_XYZ", "sekret")
	out := resolveSettings(server, "/usr/bin/testbin", "", false, false)
	if out["command"] != "/usr/bin/testbin" {
		t.Errorf("command = %v", out["command"])
	}
	env := out["env"].(map[string]any)
	if env["K"] != "sekret" {
		t.Errorf("env.K = %v", env["K"])
	}
}

func TestResolveSettingsRaw(t *testing.T) {
	server := &McpServer{
		Name: "test",
		Settings: map[string]any{
			"command": "$BINARY",
			"env":     map[string]any{"K": "$TESTKEY_MISSING"},
		},
	}
	out := resolveSettings(server, "/bin/x", "", true, false)
	env := out["env"].(map[string]any)
	if env["K"] != "$TESTKEY_MISSING" {
		t.Errorf("raw env.K = %v", env["K"])
	}
}

func TestResolveSettingsServerDir(t *testing.T) {
	server := &McpServer{
		Name: "py",
		Settings: map[string]any{
			"command": "python3",
			"args":    []any{"$SERVER_DIR/server.py"},
		},
	}
	out := resolveSettings(server, "", "/payload/dir", false, false)
	args := out["args"].([]any)
	if args[0] != "/payload/dir/server.py" {
		t.Errorf("args = %v", args)
	}
}

// --- toml ----------------------------------------------------------------------

func TestTomlString(t *testing.T) {
	if got := tomlString(`a"b\c`); got != `"a\"b\\c"` {
		t.Errorf("tomlString = %s", got)
	}
}

func TestSettingsToToml(t *testing.T) {
	settings := map[string]any{
		"command": "/bin/x",
		"args":    []any{"a", "b"},
		"env":     map[string]any{"K": "v"},
	}
	out := settingsToToml("srv", settings)
	if !strings.Contains(out, "[mcp_servers.srv]") {
		t.Errorf("no header: %s", out)
	}
	if !strings.Contains(out, `command = "/bin/x"`) {
		t.Errorf("no command: %s", out)
	}
	if !strings.Contains(out, "[mcp_servers.srv.env]") {
		t.Errorf("no env section: %s", out)
	}
}

func TestRemoveTomlServer(t *testing.T) {
	text := `[mcp_servers.one]
command = "a"

[mcp_servers.two]
command = "b"

[mcp_servers.two.env]
K = "v"
`
	out := removeTomlServer(text, "two")
	if strings.Contains(out, "mcp_servers.two") {
		t.Errorf("server still present: %s", out)
	}
	if !strings.Contains(out, "mcp_servers.one") {
		t.Errorf("other server removed: %s", out)
	}
}

func TestTomlCommandIgnoresSubtable(t *testing.T) {
	text := `[mcp_servers.srv.env]
command = "wrong"
`
	if got := tomlCommand(text, "srv"); got != "" {
		t.Errorf("command in subtable returned %q", got)
	}
	text = `[mcp_servers.srv]
command = "right"
env = {}
`
	if got := tomlCommand(text, "srv"); got != "right" {
		t.Errorf("command = %q", got)
	}
}

func TestTomlServerNames(t *testing.T) {
	text := "[mcp_servers.a]\nx=1\n[mcp_servers.b.env]\nk=2\n[other]\nz=3\n"
	got := tomlServerNames(text)
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("names = %v", got)
	}
}

// --- state ---------------------------------------------------------------------

func TestStateRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)
	data := &stateData{Version: 1}
	data.Installs = append(data.Installs, installEntry{
		Kind: "skill", Skill: "s1", Platform: "cursor", Scope: "global",
		SkillDir: "/x/skills/s1",
	})
	saveState(data)
	loaded := loadState()
	if len(loaded.Installs) != 1 || loaded.Installs[0].Skill != "s1" {
		t.Fatalf("round trip: %+v", loaded.Installs)
	}
	if loaded.Installs[0].kind() != "skill" {
		t.Error("kind()")
	}
}

func TestLegacyEntryKind(t *testing.T) {
	e := installEntry{Skill: "s"} // no Kind field — legacy skill
	if e.kind() != "skill" {
		t.Errorf("kind = %s", e.kind())
	}
	e = installEntry{Name: "m"}
	if e.kind() != "mcp" {
		t.Errorf("kind = %s", e.kind())
	}
}

func TestStillNeeded(t *testing.T) {
	ext := resolve("/home/u/.codex/agents/x.toml")
	data := &stateData{Installs: []installEntry{
		{Kind: "skill", Skill: "a", Platform: "cursor", Scope: "global",
			SkillDir:      resolve("/home/u/.cursor/skills/a"),
			ExternalFiles: []string{ext}},
	}}
	if !stillNeeded(data.Installs, ext, nil) {
		t.Error("stillNeeded = false")
	}
	if stillNeeded(data.Installs, "/other/path", nil) {
		t.Error("stillNeeded = true")
	}
	// skipping the entry that owns the path frees it
	key := data.Installs[0].key()
	if stillNeeded(data.Installs, ext, map[entryKey]bool{key: true}) {
		t.Error("stillNeeded with skip = true")
	}
}

// --- mcp payload expansion -----------------------------------------------------

func TestExpandMcpFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "mcp", "srv", "server.py"), "print(1)")
	writeFile(t, filepath.Join(root, "mcp", "srv", "lib", "util.py"), "x=1")
	server := &McpServer{
		Name: "srv",
		Path: filepath.Join(root, "mcp", "srv"),
		Root: root,
		Files: []manifestEntry{
			{from: "server.py"},
			{from: "lib"},
		},
	}
	dest := filepath.Join(t.TempDir(), "srv")
	pairs := expandMcpFiles(server, dest)
	got := map[string]bool{}
	for _, p := range pairs {
		got[p.dest] = true
	}
	for _, want := range []string{
		filepath.Join(dest, "server.py"),
		filepath.Join(dest, "lib", "util.py"),
	} {
		if !got[resolve(want)] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
}

func TestExpandMcpFilesEscapes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "mcp", "srv", "server.py"), "print(1)")
	for _, to := range []string{"../evil.py", "$HOME/evil.py"} {
		server := &McpServer{
			Name:  "srv",
			Path:  filepath.Join(root, "mcp", "srv"),
			Root:  root,
			Files: []manifestEntry{{from: "server.py", to: to}},
		}
		mustDie(t, func() { expandMcpFiles(server, filepath.Join(t.TempDir(), "srv")) })
	}
}

// --- frontmatter -----------------------------------------------------------------

func TestParseFrontmatter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	writeFile(t, path, "---\nname: foo\ndescription: a skill\nother: x\n---\nbody\n")
	fm := parseFrontmatter(path)
	if fm["name"] != "foo" || fm["description"] != "a skill" {
		t.Errorf("frontmatter = %v", fm)
	}
}

// --- plan -------------------------------------------------------------------------

func TestPlanUninstall(t *testing.T) {
	project := t.TempDir()
	target := testTarget(t, project)
	if err := os.MkdirAll(target.SkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := &stateData{Installs: []installEntry{
		{Kind: "skill", Skill: "test", Platform: target.Platform, Scope: target.Scope,
			Project: resolve(project), SkillDir: resolve(target.SkillDir)},
	}}
	m := manifest{}
	key := data.Installs[0].key()
	plan := planUninstall("test", target, &m, data, map[entryKey]bool{key: true}, false)
	var removed []string
	for _, a := range plan.Actions {
		if a.Kind == "remove" {
			removed = append(removed, a.Path)
		}
	}
	if len(removed) != 1 || removed[0] != resolve(target.SkillDir) {
		t.Errorf("removals = %v", removed)
	}
}

// --- helpers -----------------------------------------------------------------------

func TestHumanSize(t *testing.T) {
	if humanSize(500) != "500 B" && !strings.Contains(humanSize(500), "B") {
		t.Errorf("humanSize(500) = %s", humanSize(500))
	}
	if !strings.Contains(humanSize(8_400_000), "MB") {
		t.Errorf("humanSize(8.4MB) = %s", humanSize(8_400_000))
	}
}

func TestFormatVersion(t *testing.T) {
	mt, ok := fileMtime("main.go")
	if !ok {
		t.Skip("main.go mtime unavailable")
	}
	v := formatVersion(mt)
	if !strings.HasPrefix(v, "v") || len(v) != 11 {
		t.Errorf("version = %q", v)
	}
}

func TestMcpSettingsJSONRoundTrip(t *testing.T) {
	settings := map[string]any{"command": "x", "args": []any{"a"}}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back["command"] != "x" {
		t.Error("round trip failed")
	}
}

// --- force and deletion safety ---------------------------------------------------

func TestSaveStateEmptyInstalls(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)
	saveState(&stateData{Version: 1})
	raw, err := os.ReadFile(filepath.Join(tmp, "ai-tools", "installs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"installs": []`) {
		t.Fatalf("empty installs must serialize as [], got: %s", raw)
	}
	loaded := loadState() // must not die on its own empty file
	if len(loaded.Installs) != 0 {
		t.Fatalf("loaded %+v", loaded.Installs)
	}
}

func TestKeyIgnoresStaleProjectOnGlobal(t *testing.T) {
	// records written by the old tool during -gl runs carry a project on
	// global entries; a plain -g lookup must still match them
	entry := installEntry{Kind: "skill", Skill: "s", Platform: "devin",
		Scope: "global", Project: "/some/project", SkillDir: "/x/skills/s"}
	target := Target{Platform: "devin", Scope: "global", SkillDir: "/x/skills/s"}
	if findEntry([]installEntry{entry}, target.key("s"), "skill") == nil {
		t.Error("global entry with stale project not matched by -g key")
	}
	data := &stateData{Installs: []installEntry{entry}}
	dropInstall(data, "s", target, "skill")
	if len(data.Installs) != 0 {
		t.Error("dropInstall kept the entry")
	}
	// local entries still match on project
	lEntry := installEntry{Kind: "skill", Skill: "s", Platform: "devin",
		Scope: "local", Project: "/p1", SkillDir: "/p1/.devin/skills/s"}
	lTarget := Target{Platform: "devin", Scope: "local", Project: "/p2",
		SkillDir: "/p2/.devin/skills/s"}
	if findEntry([]installEntry{lEntry}, lTarget.key("s"), "skill") != nil {
		t.Error("local entries from different projects must not match")
	}
}

func TestPlanInstallForceForeignDir(t *testing.T) {
	project := t.TempDir()
	target := testTarget(t, project)
	_, skill := fixtureRepo(t, "Shared/test", `{"install": ["SKILL.md"]}`, nil)
	if err := os.MkdirAll(target.SkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(target.SkillDir, "foreign.txt"), "x")
	data := &stateData{}

	// without force: refuses
	mustDie(t, func() { planInstall(skill, target, data, nil, false, false) })
	// with force: whole dir is dropped before the copies
	plan := planInstall(skill, target, data, nil, false, true)
	if len(plan.Actions) == 0 || plan.Actions[0].Kind != "remove" ||
		plan.Actions[0].Path != target.SkillDir {
		t.Fatalf("first action = %+v", plan.Actions[0])
	}
}

func TestPlanInstallSymlinkAborts(t *testing.T) {
	project := t.TempDir()
	target := testTarget(t, project)
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(target.SkillDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, target.SkillDir); err != nil {
		t.Fatal(err)
	}
	_, skill := fixtureRepo(t, "Shared/test", `{"install": ["SKILL.md"]}`, nil)
	mustDie(t, func() { planInstall(skill, target, &stateData{}, nil, false, true) })
}

func TestPlanUninstallForceForeignDir(t *testing.T) {
	project := t.TempDir()
	target := testTarget(t, project)
	if err := os.MkdirAll(target.SkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := &stateData{}
	// no record, no force: nothing to do
	if plan := planUninstall("test", target, nil, data, nil, false); plan != nil {
		t.Fatalf("expected nil plan, got %+v", plan)
	}
	// no record + force: remove just the directory
	plan := planUninstall("test", target, nil, data, nil, true)
	if len(plan.Actions) != 1 || plan.Actions[0].Kind != "remove" ||
		plan.Actions[0].Path != target.SkillDir {
		t.Fatalf("plan = %+v", plan.Actions)
	}
}

func TestPlanUninstallSymlinkAborts(t *testing.T) {
	project := t.TempDir()
	target := testTarget(t, project)
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(target.SkillDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, target.SkillDir); err != nil {
		t.Fatal(err)
	}
	mustDie(t, func() { planUninstall("test", target, nil, &stateData{}, nil, true) })
}

func TestPlanUninstallRejectsTamperedPaths(t *testing.T) {
	project := t.TempDir()
	target := testTarget(t, project)
	if err := os.MkdirAll(target.SkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// a state record pointing outside every sanctioned root must not be removed
	data := &stateData{Installs: []installEntry{
		{Kind: "skill", Skill: "test", Platform: target.Platform, Scope: target.Scope,
			Project: resolve(project), SkillDir: resolve(target.SkillDir),
			ExternalFiles: []string{"/etc/passwd"}},
	}}
	key := data.Installs[0].key()
	mustDie(t, func() {
		planUninstall("test", target, nil, data, map[entryKey]bool{key: true}, false)
	})
}
