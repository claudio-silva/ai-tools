package main

import (
	"encoding/json"
	"os"
	"strings"
)

type installEntry struct {
	Kind          string   `json:"kind,omitempty"`
	Skill         string   `json:"skill,omitempty"`
	Name          string   `json:"name,omitempty"`
	Folder        string   `json:"folder,omitempty"`
	Platform      string   `json:"platform"`
	Scope         string   `json:"scope"`
	Project       string   `json:"project"`
	SkillDir      string   `json:"skillDir,omitempty"`
	ExternalFiles []string `json:"externalFiles,omitempty"`
	ConfigPath    string   `json:"configPath,omitempty"`
	BinaryPath    string   `json:"binaryPath,omitempty"`
	ServerDir     string   `json:"serverDir,omitempty"`
	Source        string   `json:"source,omitempty"`
}

type stateData struct {
	Version  int            `json:"version"`
	Installs []installEntry `json:"installs"`
}

func loadState() *stateData {
	path := stateFile()
	if !exists(path) {
		return &stateData{Version: 1}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		die("cannot read %s: %v", path, err)
	}
	var probe struct {
		Version  int             `json:"version"`
		Installs json.RawMessage `json:"installs"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		die("%s: %v", path, err)
	}
	var installs []installEntry
	ok := probe.Version == 1 && len(probe.Installs) > 0 &&
		probe.Installs[0] == '[' && json.Unmarshal(probe.Installs, &installs) == nil
	if !ok {
		die("%s: unrecognized install record", path)
	}
	return &stateData{Version: 1, Installs: installs}
}

func saveState(data *stateData) {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		die("cannot encode install record: %v", err)
	}
	atomicWrite(stateFile(), string(raw)+"\n")
}

func (e *installEntry) kind() string {
	if e.Kind != "" {
		return e.Kind
	}
	// legacy records carry no kind: skills use "skill", MCPs use "name"
	if e.Name != "" && e.Skill == "" {
		return "mcp"
	}
	return "skill"
}

func (e *installEntry) toolName() string {
	if e.Skill != "" {
		return e.Skill
	}
	return e.Name
}

func (e *installEntry) key() entryKey {
	return entryKey{e.toolName(), e.Platform, e.Scope, e.Project}
}

func findEntry(installs []installEntry, key entryKey, kind string) *installEntry {
	for i := range installs {
		if installs[i].kind() == kind && installs[i].key() == key {
			return &installs[i]
		}
	}
	return nil
}

func dropInstall(data *stateData, name string, target Target, kind string) {
	key := target.key(name)
	kept := data.Installs[:0]
	for _, e := range data.Installs {
		if !(e.kind() == kind && e.key() == key) {
			kept = append(kept, e)
		}
	}
	data.Installs = kept
}

// stillNeeded reports whether path is used by another recorded skill install
// outside skipKeys.
func stillNeeded(installs []installEntry, path string, skipKeys map[entryKey]bool) bool {
	text := resolve(path)
	for i := range installs {
		e := &installs[i]
		if e.kind() != "skill" {
			continue
		}
		if skipKeys[e.key()] {
			continue
		}
		skillDir := e.SkillDir
		if text == skillDir || contains(e.ExternalFiles, text) {
			return true
		}
		if skillDir != "" && strings.HasPrefix(text, strings.TrimSuffix(skillDir, "/")+"/") {
			return true
		}
	}
	return false
}

func recordSkillInstall(data *stateData, plan *Plan) {
	key := plan.Target.key(plan.Skill.Name)
	kept := data.Installs[:0]
	for _, e := range data.Installs {
		if !(e.kind() == "skill" && e.key() == key) {
			kept = append(kept, e)
		}
	}
	data.Installs = append(kept, installEntry{
		Kind:          "skill",
		Skill:         plan.Skill.Name,
		Folder:        plan.Skill.Folder,
		Platform:      plan.Target.Platform,
		Scope:         plan.Target.Scope,
		Project:       plan.Target.projectKey(),
		SkillDir:      resolve(plan.Target.SkillDir),
		ExternalFiles: plan.ExternalFiles,
		Source:        plan.Skill.Source,
	})
}

// config.json: persistent user settings (currently just the default source).
type toolConfig struct {
	Source string `json:"source,omitempty"`
}

func loadConfig() toolConfig {
	var cfg toolConfig
	raw, err := os.ReadFile(configFile())
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		die("%s: %v", configFile(), err)
	}
	return cfg
}

func saveConfig(cfg toolConfig) {
	raw, _ := json.MarshalIndent(cfg, "", "  ")
	atomicWrite(configFile(), string(raw)+"\n")
}

// loadJSONObject decodes a JSON object file; missing file → empty object.
func loadJSONObject(path string) map[string]any {
	if !exists(path) {
		return map[string]any{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		die("cannot read %s: %v", display(path), err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		die("%s: %v", display(path), err)
	}
	if data == nil {
		die("%s: expected a JSON object", display(path))
	}
	return data
}

func writeJSONObject(path string, data map[string]any) {
	raw, err := marshalJSON(data)
	if err != nil {
		die("cannot encode %s: %v", display(path), err)
	}
	atomicWrite(path, raw+"\n")
}

// marshalJSON renders an object the way Python's json.dumps(indent=2) does:
// two-space indents, no HTML escaping, UTF-8 output.
func marshalJSON(v any) (string, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}
