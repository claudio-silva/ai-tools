package main

import (
	"os"
	"path/filepath"
)

var platformOrder = []string{"cursor", "codex", "claude-code", "devin"}

var folderPlatforms = map[string][]string{
	"Shared": {"cursor", "codex", "claude-code", "devin"},
	"Cursor": {"cursor"},
	"Codex":  {"codex"},
	"Claude": {"claude-code"},
	"Devin":  {"devin"},
}

var skipDirNames = map[string]bool{"__pycache__": true, ".git": true}

// variables is ordered longest-first so $SKILLS_DIR expands before $SKILL_DIR.
var variables = []string{"SKILLS_DIR", "SKILL_DIR", "PLATFORM_HOME", "CODEX_HOME", "HOME"}

type Skill struct {
	Name      string
	Folder    string
	Path      string // directory inside the source repo
	Platforms []string
	Root      string // source repo root (local dir or cache snapshot)
	Source    string // normalized source URL/path this tool came from
}

type Target struct {
	Platform     string
	Scope        string
	Project      string // empty for global scope
	SkillsDir    string
	PlatformHome string
	SkillDir     string
}

func (t Target) projectKey() string {
	if t.Project == "" || t.Scope != "local" {
		return ""
	}
	return resolve(t.Project)
}

type entryKey struct {
	name, platform, scope, project string
}

func (t Target) key(name string) entryKey {
	return entryKey{name, t.Platform, t.Scope, t.projectKey()}
}

type Action struct {
	Kind string // copy, remove, keep
	Path string
	Src  string
	Note string
}

type Plan struct {
	Skill         Skill
	Target        Target
	Actions       []Action
	ExternalFiles []string
}

func codexHome() string {
	if raw := os.Getenv("CODEX_HOME"); raw != "" {
		return expandHome(raw)
	}
	return filepath.Join(homeDir(), ".codex")
}

func stateFile() string {
	return filepath.Join(stateRoot(), "ai-tools", "installs.json")
}

func configFile() string {
	return filepath.Join(stateRoot(), "ai-tools", "config.json")
}

func cacheRoot() string {
	return filepath.Join(stateRoot(), "ai-tools", "repos")
}

// platformPaths returns (skillsDir, platformHome) for a platform and scope.
func platformPaths(platform, scope, project string) (string, string) {
	if scope == "local" {
		if project == "" {
			die("local scope needs a project directory")
		}
		switch platform {
		case "cursor":
			return filepath.Join(project, ".cursor", "skills"), filepath.Join(project, ".cursor")
		case "codex":
			return filepath.Join(project, ".agents", "skills"), filepath.Join(project, ".codex")
		case "claude-code":
			return filepath.Join(project, ".claude", "skills"), filepath.Join(project, ".claude")
		case "devin":
			return filepath.Join(project, ".devin", "skills"), filepath.Join(project, ".devin")
		}
	}
	h := homeDir()
	switch platform {
	case "cursor":
		return filepath.Join(h, ".cursor", "skills"), filepath.Join(h, ".cursor")
	case "codex":
		return filepath.Join(codexHome(), "skills"), codexHome()
	case "claude-code":
		return filepath.Join(h, ".claude", "skills"), filepath.Join(h, ".claude")
	case "devin":
		return filepath.Join(h, ".config", "devin", "skills"), filepath.Join(h, ".config", "devin")
	}
	die("unknown platform %q", platform)
	return "", ""
}

func makeTarget(platform, scope, project, name string) Target {
	skillsDir, platformHome := platformPaths(platform, scope, project)
	return Target{
		Platform:     platform,
		Scope:        scope,
		Project:      project,
		SkillsDir:    skillsDir,
		PlatformHome: platformHome,
		SkillDir:     filepath.Join(skillsDir, name),
	}
}
