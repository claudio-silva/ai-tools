package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var placeholderRe = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

type McpServer struct {
	Name      string
	Path      string
	Platforms []string
	Settings  map[string]any
	Message   string
	Binary    string // source binary path, "" when the server ships none
	Files     []manifestEntry
	Root      string // source repo root (local dir or cache snapshot)
	Source    string // normalized source URL/path this server came from
}

func discoverMcps(root, srcURL string) []*McpServer {
	mcpRoot := filepath.Join(root, "mcp")
	if !isDir(mcpRoot) {
		return nil
	}
	var found []*McpServer
	names := map[string]bool{}
	folders, _ := os.ReadDir(mcpRoot)
	for _, folder := range folders {
		if !folder.IsDir() || strings.HasPrefix(folder.Name(), ".") {
			continue
		}
		dir := filepath.Join(mcpRoot, folder.Name())
		manifestPath := filepath.Join(dir, "manifest.json")
		if !isFile(manifestPath) {
			die("%s is missing manifest.json", relIn(root, dir))
		}
		server := loadMcpManifest(root, srcURL, dir, manifestPath)
		if names[server.Name] {
			die("duplicate MCP name %s", server.Name)
		}
		names[server.Name] = true
		found = append(found, server)
	}
	return found
}

func loadMcpManifest(root, srcURL, dir, path string) *McpServer {
	rel := relIn(root, path)
	raw, err := os.ReadFile(path)
	if err != nil {
		die("%s: %v", rel, err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		die("%s: %v", rel, err)
	}
	for key := range data {
		if key != "platforms" && key != "settings" && key != "message" && key != "files" {
			die("%s: unknown keys: %s", rel, key)
		}
	}
	var platforms []string
	for _, item := range toSlice(data["platforms"], rel, "platforms") {
		name, ok := item.(string)
		if !ok {
			die("%s: platforms must be a non-empty list of names", rel)
		}
		platforms = append(platforms, name)
	}
	if len(platforms) == 0 {
		die("%s: platforms must be a non-empty list of names", rel)
	}
	for _, name := range platforms {
		if !contains(platformOrder, name) {
			die("%s: unknown platform %s", rel, name)
		}
	}
	settings, ok := data["settings"].(map[string]any)
	if !ok || len(settings) == 0 {
		die("%s: settings must be a non-empty object", rel)
	}
	message, _ := data["message"].(string)
	if strings.TrimSpace(message) == "" {
		die("%s: message must be a non-empty string", rel)
	}
	name := filepath.Base(dir)
	binary := filepath.Join(dir, "bin", name)
	hasBinary := isFile(binary)
	if !hasBinary {
		binary = ""
	}
	var files []manifestEntry
	for _, item := range toSlice(data["files"], rel, "files") {
		switch v := item.(type) {
		case string:
			if v == "" {
				die("%s: empty files path", rel)
			}
			files = append(files, manifestEntry{from: v})
		case map[string]any:
			from, ok1 := v["from"].(string)
			to, ok2 := v["to"].(string)
			if len(v) != 2 || !ok1 || !ok2 || from == "" || to == "" {
				die("%s: files objects need string from and to", rel)
			}
			files = append(files, manifestEntry{from: from, to: to})
		default:
			die("%s: files entries must be strings or objects", rel)
		}
	}
	for _, ph := range collectPlaceholders(settings) {
		if ph == "BINARY" && !hasBinary {
			die("%s: settings use $BINARY but bin/%s is missing", rel, name)
		}
		if ph == "SERVER_DIR" && len(files) == 0 {
			die("%s: settings use $SERVER_DIR but no files are shipped", rel)
		}
	}
	return &McpServer{
		Name:      name,
		Path:      dir,
		Platforms: platforms,
		Settings:  settings,
		Message:   strings.TrimSpace(message),
		Binary:    binary,
		Files:     files,
		Root:      root,
		Source:    srcURL,
	}
}

func toSlice(v any, rel, key string) []any {
	if v == nil {
		return nil
	}
	s, ok := v.([]any)
	if !ok {
		die("%s: %s must be a list", rel, key)
	}
	return s
}

func mcpConfigPath(platform, scope, project string) string {
	if scope == "local" {
		if project == "" {
			die("local scope needs a project directory")
		}
		switch platform {
		case "cursor":
			return filepath.Join(project, ".cursor", "mcp.json")
		case "codex":
			return filepath.Join(project, ".codex", "config.toml")
		case "claude-code":
			return filepath.Join(project, ".mcp.json")
		default:
			return filepath.Join(project, ".devin", "mcp_config.json")
		}
	}
	switch platform {
	case "cursor":
		return filepath.Join(homeDir(), ".cursor", "mcp.json")
	case "codex":
		return filepath.Join(codexHome(), "config.toml")
	case "claude-code":
		return filepath.Join(homeDir(), ".claude.json")
	default:
		return filepath.Join(homeDir(), ".config", "devin", "mcp_config.json")
	}
}

func binaryDir() string {
	for _, dir := range setupDirs() {
		if isDir(dir) && writable(dir) {
			return dir
		}
	}
	die("neither ~/bin nor ~/.local/bin exists and is writable")
	return ""
}

func writable(dir string) bool {
	probe, err := os.CreateTemp(dir, ".aitools-")
	if err != nil {
		return false
	}
	probe.Close()
	os.Remove(probe.Name())
	return true
}

func binaryDest(server *McpServer) string {
	return filepath.Join(binaryDir(), server.Name)
}

// serverDir is the managed payload directory for file-shipping MCP servers.
func serverDir(name string) string {
	return filepath.Join(dataRoot(), "ai-tools", "mcp", name)
}

func collectPlaceholders(value any) []string {
	var names []string
	switch v := value.(type) {
	case string:
		for _, m := range placeholderRe.FindAllStringSubmatch(v, -1) {
			names = append(names, m[1])
		}
	case map[string]any:
		for _, item := range v {
			names = append(names, collectPlaceholders(item)...)
		}
	case []any:
		for _, item := range v {
			names = append(names, collectPlaceholders(item)...)
		}
	}
	return uniqueStrings(names)
}

func replacePlaceholders(value any, mapping map[string]string) any {
	switch v := value.(type) {
	case string:
		return placeholderRe.ReplaceAllStringFunc(v, func(match string) string {
			name := match[1:]
			if sub, ok := mapping[name]; ok {
				return sub
			}
			return match
		})
	case map[string]any:
		out := map[string]any{}
		for key, item := range v {
			out[key] = replacePlaceholders(item, mapping)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = replacePlaceholders(item, mapping)
		}
		return out
	}
	return value
}

func resolveSettings(server *McpServer, commandPath, payloadDir string, raw, dryRun bool) map[string]any {
	mapping := map[string]string{}
	if commandPath != "" {
		mapping["BINARY"] = resolve(commandPath)
	}
	if payloadDir != "" {
		mapping["SERVER_DIR"] = payloadDir
	}
	var names []string
	for _, name := range collectPlaceholders(server.Settings) {
		if name != "BINARY" && name != "SERVER_DIR" {
			names = append(names, name)
		}
	}
	var missing []string
	for _, name := range names {
		if raw {
			continue
		}
		if value := os.Getenv(name); value != "" {
			mapping[name] = value
			continue
		}
		missing = append(missing, name)
	}
	if len(missing) > 0 && dryRun {
		var refs []string
		for _, n := range missing {
			refs = append(refs, "$"+n)
		}
		fmt.Println("  would prompt for " + strings.Join(refs, ", "))
	} else if len(missing) > 0 {
		if !isTTY(os.Stdin) {
			die("%s: set %s in the environment, or pass --raw to write placeholders",
				server.Name, strings.Join(missing, ", "))
		}
		for _, name := range missing {
			if value := hiddenPrompt(fmt.Sprintf("  %s: ", name)); value != "" {
				mapping[name] = value
			}
		}
	}
	return replacePlaceholders(deepCopy(server.Settings), mapping).(map[string]any)
}

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, item := range t {
			out[k] = deepCopy(item)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = deepCopy(item)
		}
		return out
	}
	return v
}

// --- TOML (codex config) -----------------------------------------------------

func tomlString(value string) string {
	return "\"" + strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "\"", "\\\"") + "\""
}

func tomlValue(value any) string {
	switch v := value.(type) {
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64, int, int64:
		return fmt.Sprintf("%v", v)
	case []any:
		var parts []string
		for _, item := range v {
			parts = append(parts, tomlValue(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return tomlString(fmt.Sprintf("%v", value))
}

func settingsToToml(name string, settings map[string]any) string {
	lines := []string{fmt.Sprintf("[mcp_servers.%s]", name)}
	nested := map[string]map[string]any{}
	for _, key := range sortedKeys(settings) {
		value := settings[key]
		if sub, ok := value.(map[string]any); ok {
			nested[key] = sub
			continue
		}
		lines = append(lines, fmt.Sprintf("%s = %s", key, tomlValue(value)))
	}
	for _, nestedKey := range sortedKeys(nested) {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("[mcp_servers.%s.%s]", name, nestedKey))
		for _, key := range sortedKeys(nested[nestedKey]) {
			lines = append(lines, fmt.Sprintf("%s = %s", key, tomlValue(nested[nestedKey][key])))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// isMcpSectionHeader matches `[mcp_servers.<name>]` or `[mcp_servers.<name>.sub]`.
func isMcpSectionHeader(line, name string) bool {
	re := regexp.MustCompile(`^\[mcp_servers\.` + regexp.QuoteMeta(name) + `(\.[^\]]*)?\]`)
	return re.MatchString(line)
}

var mcpSectionRe = regexp.MustCompile(`(?m)^\[mcp_servers\.([A-Za-z0-9_-]+)(\.[^\]]+)?\]\s*$`)

// removeTomlServer drops the [mcp_servers.<name>] section (and its subtables).
func removeTomlServer(text, name string) string {
	var out []string
	skipping := false
	for _, line := range strings.SplitAfter(text, "\n") {
		if isMcpSectionHeader(strings.TrimRight(line, "\n"), name) {
			skipping = true
			continue
		}
		if skipping && strings.HasPrefix(line, "[") {
			skipping = false
		}
		if !skipping {
			out = append(out, line)
		}
	}
	cleaned := strings.Join(out, "")
	for strings.Contains(cleaned, "\n\n\n") {
		cleaned = strings.ReplaceAll(cleaned, "\n\n\n", "\n\n")
	}
	cleaned = strings.TrimSpace(cleaned)
	if cleaned != "" {
		cleaned += "\n"
	}
	return cleaned
}

func tomlServerNames(text string) []string {
	var names []string
	for _, m := range mcpSectionRe.FindAllStringSubmatch(text, -1) {
		if !contains(names, m[1]) {
			names = append(names, m[1])
		}
	}
	return names
}

// tomlCommand returns the command value in [mcp_servers.<name>], only from the
// main section — a command in a subtable like [mcp_servers.<name>.env] does
// not count.
func tomlCommand(text, name string) string {
	header := "[mcp_servers." + name + "]"
	inSection := false
	commandRe := regexp.MustCompile(`^command\s*=\s*"([^"]*)"`)
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "[") {
			inSection = strings.TrimSpace(line) == header
			continue
		}
		if inSection {
			if m := commandRe.FindStringSubmatch(line); m != nil {
				return m[1]
			}
		}
	}
	return ""
}

// --- JSON configs ------------------------------------------------------------

func jsonServers(path string) map[string]any {
	data := loadJSONObject(path)
	servers, ok := data["mcpServers"]
	if !ok || servers == nil {
		return map[string]any{}
	}
	obj, ok := servers.(map[string]any)
	if !ok {
		die("%s: mcpServers must be an object", display(path))
	}
	return obj
}

func configHasServer(path, name string) bool {
	if !exists(path) {
		return false
	}
	if strings.HasSuffix(path, ".toml") {
		return contains(tomlServerNames(readFile(path)), name)
	}
	_, ok := jsonServers(path)[name]
	return ok
}

func configCommand(path, name string) string {
	if !exists(path) {
		return ""
	}
	if strings.HasSuffix(path, ".toml") {
		return tomlCommand(readFile(path), name)
	}
	entry, ok := jsonServers(path)[name].(map[string]any)
	if !ok {
		return ""
	}
	command, _ := entry["command"].(string)
	return command
}

func writeJSONServer(path, name string, settings map[string]any) {
	data := loadJSONObject(path)
	servers, ok := data["mcpServers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
		data["mcpServers"] = servers
	}
	servers[name] = settings
	writeJSONObject(path, data)
}

func removeJSONServer(path, name string) {
	if !exists(path) {
		return
	}
	data := loadJSONObject(path)
	servers, ok := data["mcpServers"].(map[string]any)
	if !ok {
		return
	}
	delete(servers, name)
	writeJSONObject(path, data)
}

func writeTomlServer(path, name string, settings map[string]any) {
	text := ""
	if exists(path) {
		text = readFile(path)
	}
	text = removeTomlServer(text, name)
	block := settingsToToml(name, settings)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" && !strings.HasSuffix(text, "\n\n") {
		text += "\n"
	}
	atomicWrite(path, text+block)
}

func removeTomlServerFile(path, name string) {
	if !exists(path) {
		return
	}
	atomicWrite(path, removeTomlServer(readFile(path), name))
}

func writeServer(path, name string, settings map[string]any) {
	if strings.HasSuffix(path, ".toml") {
		writeTomlServer(path, name, settings)
	} else {
		writeJSONServer(path, name, settings)
	}
}

func removeServer(path, name string) {
	if strings.HasSuffix(path, ".toml") {
		removeTomlServerFile(path, name)
	} else {
		removeJSONServer(path, name)
	}
}

// --- binary / payload ---------------------------------------------------------

func copyBinary(server *McpServer, dest string, dryRun bool, verb string) {
	if resolve(dest) == resolve(server.Binary) {
		return
	}
	fmt.Printf("  %s %s\n", verb, display(dest))
	if dryRun {
		return
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		die("cannot create %s: %v", display(filepath.Dir(dest)), err)
	}
	copyFile(server.Binary, dest)
	st, err := os.Stat(dest)
	if err == nil {
		os.Chmod(dest, st.Mode()|0o111)
	}
	if mtime, ok := fileMtime(server.Binary); ok {
		os.Chtimes(dest, mtime, mtime)
	}
}

func copiedToHomeBin(path string) bool {
	parent := filepath.Dir(resolve(path))
	for _, dir := range setupDirs() {
		if parent == resolve(dir) {
			return true
		}
	}
	return false
}

func binaryStillNeeded(data *stateData, path string, skipKeys map[entryKey]bool) bool {
	wanted := resolve(expandHome(path))
	for i := range data.Installs {
		e := &data.Installs[i]
		if e.kind() != "mcp" || skipKeys[e.key()] {
			continue
		}
		if e.BinaryPath != "" && resolve(expandHome(e.BinaryPath)) == wanted {
			return true
		}
	}
	return false
}

// binaryRecorded reports whether some install record owns this binary path.
func binaryRecorded(data *stateData, path string) bool {
	for _, e := range data.Installs {
		if e.BinaryPath != "" && resolve(e.BinaryPath) == resolve(path) {
			return true
		}
	}
	return false
}

// serverDirRecorded reports whether some install record owns this payload dir.
func serverDirRecorded(data *stateData, path string) bool {
	for _, e := range data.Installs {
		if e.ServerDir != "" && resolve(e.ServerDir) == resolve(path) {
			return true
		}
	}
	return false
}

// guardForeignMcpDests refuses to touch a binary or payload destination the
// tool did not install. With force, a foreign payload dir is reported so the
// caller can clear it before copying; foreign binary files are overwritten.
// Returns true when payload exists but is not recorded.
func guardForeignMcpDests(data *stateData, srcBinary, dest, payload string, force bool) bool {
	if dest != "" && resolve(dest) != resolve(srcBinary) {
		switch {
		case isSymlink(dest):
			die("%s is a symlink; refusing to install over it. Remove the link manually.", displayLink(dest))
		case isDir(dest):
			die("%s is a directory; refusing to install over it", display(dest))
		case exists(dest) && !binaryRecorded(data, dest) && !force:
			die("%s already exists and was not installed by this tool. Move it aside, then run again.",
				display(dest))
		}
	}
	payloadForeign := false
	if payload != "" {
		switch {
		case isSymlink(payload):
			die("%s is a symlink; refusing to install over it. Remove the link manually.", displayLink(payload))
		case isDir(payload) && !serverDirRecorded(data, payload):
			if !force {
				die("%s already exists and was not installed by this tool. Move it aside, then run again.",
					display(payload))
			}
			payloadForeign = true
		}
	}
	return payloadForeign
}

func serverDirStillNeeded(data *stateData, path string, skipKeys map[entryKey]bool) bool {
	wanted := resolve(path)
	for i := range data.Installs {
		e := &data.Installs[i]
		if e.kind() != "mcp" || skipKeys[e.key()] {
			continue
		}
		if e.ServerDir != "" && resolve(e.ServerDir) == wanted {
			return true
		}
	}
	return false
}

// expandMcpFiles resolves the manifest's files list to src→dest pairs under
// serverDir, using the same source rules as skill manifests.
func expandMcpFiles(server *McpServer, destDir string) []filePair {
	vars := map[string]string{"SERVER_DIR": destDir, "HOME": homeDir()}
	var pairs []filePair
	seen := map[string]string{}

	add := func(src, dest string) {
		resolvedSrc := resolve(src)
		if !isInside(resolvedSrc, resolve(server.Path)) {
			die("%s: source escapes the server directory (%s)", server.Name, src)
		}
		resolvedDest := resolve(dest)
		if !isInside(resolvedDest, resolve(destDir)) || resolvedDest == resolve(destDir) {
			die("%s: destination must stay inside the server directory (%s)", server.Name, dest)
		}
		if prev, ok := seen[resolvedDest]; ok && prev != resolvedSrc {
			die("%s: %s is installed twice", server.Name, display(resolvedDest))
		}
		seen[resolvedDest] = resolvedSrc
		pairs = append(pairs, filePair{resolvedSrc, resolvedDest})
	}

	for _, entry := range server.Files {
		if entry.to == "" {
			sourceOK(server.Name, entry.from, false)
			src := filepath.Join(server.Path, entry.from)
			if !exists(src) {
				die("%s: missing %s", server.Name, entry.from)
			}
			for _, file := range filesUnder(server.Name, server.Path, src) {
				rel, _ := filepath.Rel(server.Path, file)
				add(file, filepath.Join(destDir, rel))
			}
			continue
		}
		destText := entry.to
		for _, token := range []string{"SERVER_DIR", "HOME"} {
			destText = strings.ReplaceAll(destText, "$"+token, vars[token])
		}
		if strings.Contains(destText, "$") {
			die("%s: unknown variable in files.to (%s)", server.Name, entry.to)
		}
		if !filepath.IsAbs(destText) {
			destText = filepath.Join(destDir, destText)
		}
		if hasGlobChars(entry.from) {
			sourceOK(server.Name, entry.from, true)
			matches, _ := filepath.Glob(filepath.Join(server.Path, entry.from))
			var files []string
			for _, match := range matches {
				if isFile(match) {
					rel, _ := filepath.Rel(server.Path, match)
					if !ignoredFile(rel) {
						files = append(files, match)
					}
				}
			}
			if len(files) == 0 {
				die("%s: no files matched %s", server.Name, entry.from)
			}
			sort.Strings(files)
			for _, file := range files {
				add(file, filepath.Join(destText, filepath.Base(file)))
			}
			continue
		}
		sourceOK(server.Name, entry.from, false)
		src := filepath.Join(server.Path, entry.from)
		if !exists(src) {
			die("%s: missing %s", server.Name, entry.from)
		}
		if isDir(src) {
			for _, file := range filesUnder(server.Name, server.Path, src) {
				rel, _ := filepath.Rel(src, file)
				add(file, filepath.Join(destText, rel))
			}
			continue
		}
		dest := destText
		if strings.HasSuffix(destText, "/") || isDir(destText) {
			dest = filepath.Join(destText, filepath.Base(src))
		}
		add(src, dest)
	}
	return pairs
}

// copyPayload installs a server's files into destDir, removing stale files.
func copyPayload(server *McpServer, destDir string, dryRun bool, verb string) {
	pairs := expandMcpFiles(server, destDir)
	keep := map[string]bool{}
	for _, p := range pairs {
		// p.dest is resolved; destDir must be resolved too or rel is garbage
		// and emptyDirs would treat real payload dirs as stale
		rel, _ := filepath.Rel(resolve(destDir), p.dest)
		keep[filepath.ToSlash(rel)] = true
	}
	if !dryRun {
		for _, stale := range staleFiles(destDir, keep) {
			trashPath(stale)
		}
	}
	for _, p := range pairs {
		fmt.Printf("  %s %s\n", verb, display(p.dest))
		if dryRun {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p.dest), 0o755); err != nil {
			die("cannot create %s: %v", display(filepath.Dir(p.dest)), err)
		}
		copyFile(p.src, p.dest)
	}
	if !dryRun {
		for _, dir := range emptyDirs(destDir, keep) {
			trashPath(dir)
		}
	}
}

func recordMcpInstall(data *stateData, server *McpServer, target Target, config, binary, payloadDir string) {
	dropInstall(data, server.Name, target, "mcp")
	data.Installs = append(data.Installs, installEntry{
		Kind:       "mcp",
		Name:       server.Name,
		Platform:   target.Platform,
		Scope:      target.Scope,
		Project:    target.projectKey(),
		ConfigPath: resolve(config),
		BinaryPath: resolveOrEmpty(binary),
		ServerDir:  payloadDir,
		Source:     server.Source,
	})
}

func resolveOrEmpty(path string) string {
	if path == "" {
		return ""
	}
	return resolve(path)
}

func mcpsByName(servers []*McpServer) map[string]*McpServer {
	m := map[string]*McpServer{}
	for _, s := range servers {
		m[s.Name] = s
	}
	return m
}

func platformsForMcp(server *McpServer, filter []string, strict bool) []string {
	if filter == nil {
		return server.Platforms
	}
	var unsupported []string
	for _, name := range filter {
		if !contains(server.Platforms, name) {
			unsupported = append(unsupported, name)
		}
	}
	if len(unsupported) > 0 && strict {
		die("%s cannot target %s; it supports %s",
			server.Name, strings.Join(unsupported, ", "), strings.Join(server.Platforms, ", "))
	}
	var out []string
	for _, p := range server.Platforms {
		if contains(filter, p) {
			out = append(out, p)
		}
	}
	return out
}

func resolveMcpNames(servers []*McpServer, names []string) {
	known := mcpsByName(servers)
	var missing []string
	for _, name := range names {
		if _, ok := known[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		die("unknown MCP: %s", strings.Join(missing, ", "))
	}
}

func resolveMcpRequested(servers []*McpServer, names []string, data *stateData) {
	known := map[string]bool{}
	for _, s := range servers {
		known[s.Name] = true
	}
	for i := range data.Installs {
		if data.Installs[i].kind() == "mcp" && data.Installs[i].Name != "" {
			known[data.Installs[i].Name] = true
		}
	}
	var missing []string
	for _, name := range names {
		if !known[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		die("unknown MCP: %s", strings.Join(missing, ", "))
	}
}

func printMessage(server *McpServer) {
	fmt.Println()
	for _, line := range wrapText(server.Message, 76) {
		fmt.Println(line)
	}
}

// --- commands -----------------------------------------------------------------

func mcpCmdInstall(args *cliArgs, cat *catalog, names []string, data *stateData, allowEmpty bool) {
	scopes := selectedScopes(args, "global")
	platformFilter := selectedPlatforms(args.platforms)
	var project string
	if contains(scopes, "local") {
		project = projectDir(args)
	}
	if args.directory != "" && !contains(scopes, "local") {
		die("--directory is used with --local")
	}
	byName := mcpsByName(cat.mcps)
	raw := args.raw
	type choice struct {
		server    *McpServer
		platforms []string
	}
	var chosen []choice
	if args.all {
		for _, server := range cat.mcps {
			if platforms := platformsForMcp(server, platformFilter, false); len(platforms) > 0 {
				chosen = append(chosen, choice{server, platforms})
			}
		}
		if len(chosen) == 0 {
			if allowEmpty {
				return
			}
			die("no tools match the selected platforms")
		}
	} else {
		resolveMcpNames(cat.mcps, names)
		for _, name := range names {
			server := byName[name]
			chosen = append(chosen, choice{server, platformsForMcp(server, platformFilter, true)})
		}
	}

	type job struct {
		server         *McpServer
		dest           string
		payload        string
		payloadForeign bool
		destinations   []struct {
			target Target
			config string
		}
	}
	var jobs []job
	for _, c := range chosen {
		j := job{server: c.server}
		if c.server.Binary != "" {
			j.dest = binaryDest(c.server)
		}
		if len(c.server.Files) > 0 {
			j.payload = serverDir(c.server.Name)
		}
		j.payloadForeign = guardForeignMcpDests(data, c.server.Binary, j.dest, j.payload, args.force)
		for _, scope := range scopes {
			for _, platform := range c.platforms {
				target := makeTarget(platform, scope, project, c.server.Name)
				config := mcpConfigPath(platform, scope, project)
				current := findEntry(data.Installs, target.key(c.server.Name), "mcp")
				if configHasServer(config, c.server.Name) && current == nil && !args.force {
					die("%s already exists in %s and was not installed by this tool",
						c.server.Name, display(config))
				}
				j.destinations = append(j.destinations, struct {
					target Target
					config string
				}{target, config})
			}
		}
		jobs = append(jobs, j)
	}

	if args.dryRun {
		fmt.Println("dry-run: no files will be changed")
	}
	for _, j := range jobs {
		settings := resolveSettings(j.server, j.dest, j.payload, raw, args.dryRun)
		verb := "installed"
		if args.dryRun {
			verb = "would install"
		}
		if j.dest != "" {
			copyBinary(j.server, j.dest, args.dryRun, verb)
		}
		if j.payload != "" {
			if j.payloadForeign {
				fmt.Printf("  remove %s\n", display(j.payload))
				if !args.dryRun {
					trashPath(j.payload)
				}
			}
			copyPayload(j.server, j.payload, args.dryRun, verb)
		}
		for _, d := range j.destinations {
			fmt.Printf("  %s %s → %s (%s)\n", verb, j.server.Name, d.target.Platform, d.target.Scope)
			action := "edited"
			if args.dryRun {
				action = "would edit"
			}
			fmt.Printf("  %s %s\n", action, display(d.config))
			if !args.dryRun {
				writeServer(d.config, j.server.Name, settings)
				recordMcpInstall(data, j.server, d.target, d.config, j.dest, j.payload)
				saveState(data)
			}
		}
		printMessage(j.server)
	}
}

func mcpCmdUninstall(args *cliArgs, cat *catalog, names []string, data *stateData, allowEmpty bool) {
	scopes := selectedScopes(args, "global")
	platformFilter := selectedPlatforms(args.platforms)
	var project string
	if contains(scopes, "local") {
		project = projectDir(args)
	}
	if args.directory != "" && !contains(scopes, "local") {
		die("--directory is used with --local")
	}
	byName := mcpsByName(cat.mcps)
	type selection struct {
		name    string
		target  Target
		config  string
		binary  string
		payload string
	}
	var selected []selection
	if args.all {
		for i := range data.Installs {
			e := &data.Installs[i]
			if e.kind() != "mcp" || !contains(scopes, e.Scope) {
				continue
			}
			if platformFilter != nil && !contains(platformFilter, e.Platform) {
				continue
			}
			if e.Scope == "local" && e.Project != project {
				continue
			}
			p := ""
			if e.Scope == "local" {
				p = project
			}
			target := makeTarget(e.Platform, e.Scope, p, e.Name)
			selected = append(selected, selection{e.Name, target, e.ConfigPath, e.BinaryPath, e.ServerDir})
		}
	} else {
		if !args.force {
			resolveMcpRequested(cat.mcps, names, data)
		}
		for _, name := range names {
			server := byName[name]
			platforms := platformOrder
			if server != nil {
				platforms = platformsForMcp(server, platformFilter, true)
			} else if platformFilter != nil {
				platforms = platformFilter
			}
			matched := false
			for _, scope := range scopes {
				p := ""
				if scope == "local" {
					p = project
				}
				for _, platform := range platforms {
					target := makeTarget(platform, scope, project, name)
					config := mcpConfigPath(platform, scope, p)
					entry := findEntry(data.Installs, target.key(name), "mcp")
					if entry != nil {
						selected = append(selected, selection{name, target, entry.ConfigPath, entry.BinaryPath, entry.ServerDir})
						matched = true
					} else if args.force && configHasServer(config, name) {
						// foreign entry: only the config key is known, so only
						// that is removed — any binary or payload is left alone.
						selected = append(selected, selection{name, target, config, "", ""})
						matched = true
					}
				}
			}
			if !matched {
				fmt.Printf("%s is not installed\n", name)
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
	if args.dryRun {
		fmt.Println("dry-run: no files will be changed")
	}
	for _, s := range selected {
		verb := "uninstalled"
		if args.dryRun {
			verb = "would uninstall"
		}
		fmt.Printf("%s %s → %s (%s)\n", verb, s.name, s.target.Platform, s.target.Scope)
		fmt.Printf("  remove %s %s\n", display(s.config), s.name)
		if !args.dryRun {
			removeServer(s.config, s.name)
		}
		// Only files this tool installed are removed: the recorded binary must
		// live inside ~/bin or ~/.local/bin, and the recorded payload inside the
		// managed MCP directory. Foreign entries only lose the config key.
		if s.binary != "" && !copiedToHomeBin(s.binary) {
			fmt.Printf("  keep %s (outside %s — not ours)\n", display(s.binary), display(setupDirs()[0]))
		} else if s.binary != "" {
			if binaryStillNeeded(data, s.binary, skipKeys) {
				fmt.Printf("  keep %s (still used by another install)\n", display(s.binary))
			} else if present(s.binary) {
				fmt.Printf("  remove %s\n", display(s.binary))
				if !args.dryRun {
					trashPath(s.binary)
				}
			}
		}
		if s.payload != "" {
			managed := filepath.Join(dataRoot(), "ai-tools", "mcp")
			switch {
			case !isInside(resolve(s.payload), resolve(managed)) || resolve(s.payload) == resolve(managed):
				fmt.Printf("  keep %s (outside the managed MCP directory — not ours)\n", display(s.payload))
			case isSymlink(s.payload):
				die("%s is a symlink; refusing to remove it. Remove the link manually.", displayLink(s.payload))
			case serverDirStillNeeded(data, s.payload, skipKeys):
				fmt.Printf("  keep %s (still used by another install)\n", display(s.payload))
			case present(s.payload):
				fmt.Printf("  remove %s\n", display(s.payload))
				if !args.dryRun {
					trashPath(s.payload)
				}
			}
		}
		if !args.dryRun {
			dropInstall(data, s.name, s.target, "mcp")
			saveState(data)
		}
	}
}

// mcpIsNewer reports whether the source copy is newer than the install.
func mcpIsNewer(server *McpServer, dest, payload, config string, entry *installEntry) bool {
	if !configHasServer(config, server.Name) {
		return false
	}
	if server.Binary != "" {
		if destSt, err := os.Stat(dest); err == nil {
			if srcSt, err := os.Stat(server.Binary); err == nil && srcSt.ModTime().After(destSt.ModTime()) {
				return true
			}
		}
	}
	if len(server.Files) > 0 {
		for _, p := range expandMcpFiles(server, payload) {
			st, err := os.Stat(p.dest)
			srcSt, srcErr := os.Stat(p.src)
			if err != nil || (srcErr == nil && srcSt.ModTime().After(st.ModTime())) {
				return true
			}
		}
	}
	command := configCommand(config, server.Name)
	if command != "" && dest != "" {
		if resolve(expandHome(command)) != resolve(dest) {
			return true
		}
	}
	if entry != nil && entry.BinaryPath != "" && dest != "" {
		if resolve(expandHome(entry.BinaryPath)) != resolve(dest) {
			return true
		}
	}
	return false
}

func mcpCmdUpdate(args *cliArgs, cat *catalog, names []string, data *stateData, allowEmpty bool) bool {
	scopes := selectedScopes(args, "global")
	platformFilter := selectedPlatforms(args.platforms)
	var project string
	if contains(scopes, "local") {
		project = projectDir(args)
	}
	if args.directory != "" && !contains(scopes, "local") {
		die("--directory is used with --local")
	}
	byName := mcpsByName(cat.mcps)
	var chosen []*McpServer
	if len(names) > 0 {
		resolveMcpNames(cat.mcps, names)
		for _, name := range names {
			chosen = append(chosen, byName[name])
		}
	} else {
		chosen = cat.mcps
	}
	raw := args.raw

	type outdatedJob struct {
		server  *McpServer
		target  Target
		config  string
		dest    string
		payload string
	}
	var outdated []outdatedJob
	var current []struct {
		server *McpServer
		target Target
	}
	installedNames := map[string]bool{}
	for _, server := range chosen {
		platforms := platformsForMcp(server, platformFilter, len(names) > 0)
		if len(platforms) == 0 {
			continue
		}
		dest := ""
		if server.Binary != "" {
			dest = binaryDest(server)
		}
		payload := ""
		if len(server.Files) > 0 {
			payload = serverDir(server.Name)
		}
		for _, scope := range scopes {
			for _, platform := range platforms {
				target := makeTarget(platform, scope, project, server.Name)
				config := mcpConfigPath(platform, scope, project)
				if !configHasServer(config, server.Name) {
					continue
				}
				installedNames[server.Name] = true
				entry := findEntry(data.Installs, target.key(server.Name), "mcp")
				if mcpIsNewer(server, dest, payload, config, entry) {
					// update never deletes foreign files either: config entries
					// may be adopted, but an unrecorded binary/payload path aborts
					guardForeignMcpDests(data, server.Binary, dest, payload, false)
					outdated = append(outdated, outdatedJob{server, target, config, dest, payload})
				} else if len(names) > 0 {
					current = append(current, struct {
						server *McpServer
						target Target
					}{server, target})
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
			fmt.Printf("%s is up to date for %s (%s)\n", c.server.Name, c.target.Platform, c.target.Scope)
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
	settingsByName := map[string]map[string]any{}
	copied := map[string]bool{}
	messaged := map[string]bool{}
	for _, j := range outdated {
		verb := "updated"
		if args.dryRun {
			verb = "would update"
		}
		fmt.Printf("  %s %s → %s (%s)\n", verb, j.server.Name, j.target.Platform, j.target.Scope)
		fmt.Printf("    remove %s %s\n", display(j.config), j.server.Name)
		if !args.dryRun {
			removeServer(j.config, j.server.Name)
			dropInstall(data, j.server.Name, j.target, "mcp")
			saveState(data)
		}
		if !copied[j.server.Name] {
			if j.dest != "" {
				copyBinary(j.server, j.dest, args.dryRun, verb)
			}
			if j.payload != "" {
				copyPayload(j.server, j.payload, args.dryRun, verb)
			}
			copied[j.server.Name] = true
		}
		if settingsByName[j.server.Name] == nil {
			settingsByName[j.server.Name] = resolveSettings(j.server, j.dest, j.payload, raw, args.dryRun)
		}
		if !args.dryRun {
			writeServer(j.config, j.server.Name, settingsByName[j.server.Name])
			recordMcpInstall(data, j.server, j.target, j.config, j.dest, j.payload)
			saveState(data)
		}
		if !messaged[j.server.Name] {
			printMessage(j.server)
			messaged[j.server.Name] = true
		}
	}
	return true
}

func mcpInstallStatus(server *McpServer, data *stateData) [][3]string {
	project := resolve(cwd())
	dest := ""
	if server.Binary != "" {
		dest = binaryDest(server)
	}
	payload := ""
	if len(server.Files) > 0 {
		payload = serverDir(server.Name)
	}
	var found [][3]string
	for _, scope := range []string{"global", "local"} {
		for _, platform := range server.Platforms {
			p := ""
			if scope == "local" {
				p = project
			}
			config := mcpConfigPath(platform, scope, p)
			if !configHasServer(config, server.Name) {
				continue
			}
			target := makeTarget(platform, scope, p, server.Name)
			entry := findEntry(data.Installs, target.key(server.Name), "mcp")
			mark := "◎"
			if entry != nil {
				mark = "◉"
				if mcpIsNewer(server, dest, payload, config, entry) {
					mark = "▲"
				}
			}
			found = append(found, [3]string{scope, platform, mark})
		}
	}
	return found
}

func mcpRepoMtime(server *McpServer) time.Time {
	var latest time.Time
	consider := func(path string) {
		if st, err := os.Stat(path); err == nil && st.ModTime().After(latest) {
			latest = st.ModTime()
		}
	}
	if server.Binary != "" {
		consider(server.Binary)
	}
	for _, p := range expandMcpFiles(server, serverDir(server.Name)) {
		consider(p.src)
	}
	if latest.IsZero() {
		consider(filepath.Join(server.Path, "manifest.json"))
	}
	return latest
}

func mcpCmdAbout(args *cliArgs, cat *catalog, name string, data *stateData) {
	server := mcpsByName(cat.mcps)[name]
	fields := [][2]string{
		{"version", formatVersion(mcpRepoMtime(server))},
		{"platforms", strings.Join(server.Platforms, ", ")},
		{"path", relIn(server.Root, server.Path)},
	}
	if server.Binary != "" {
		fields = append(fields, [2]string{"binary", relIn(server.Root, server.Binary)})
	}
	printAbout(server.Name, "MCP server", fields, mcpInstallStatus(server, data), server.Message)
}

func collectInstalledMcps(args *cliArgs, cat *catalog, names []string, data *stateData, scopes []string, project string) []installedGroup {
	nameFilter := map[string]bool{}
	for _, n := range names {
		nameFilter[n] = true
	}
	byName := mcpsByName(cat.mcps)
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
			config := mcpConfigPath(platform, scope, scopeProject)
			group := installedGroup{Scope: scope, Platform: platform, Location: config}
			groups = append(groups, group)
			var onDisk []string
			if exists(config) {
				if strings.HasSuffix(config, ".toml") {
					onDisk = tomlServerNames(readFile(config))
				} else {
					onDisk = sortedKeys(jsonServers(config))
				}
			}
			var recorded []installEntry
			for i := range data.Installs {
				e := &data.Installs[i]
				if e.kind() != "mcp" || e.Platform != platform || e.Scope != scope {
					continue
				}
				if scope == "local" && e.Project != project {
					continue
				}
				recorded = append(recorded, *e)
			}
			nameSet := map[string]bool{}
			for _, n := range onDisk {
				nameSet[n] = true
			}
			for _, e := range recorded {
				nameSet[e.Name] = true
			}
			var all []string
			for n := range nameSet {
				all = append(all, n)
			}
			sort.Strings(all)
			if len(nameFilter) > 0 {
				var filtered []string
				for _, n := range all {
					if nameFilter[n] {
						filtered = append(filtered, n)
					}
				}
				all = filtered
			}
			if len(all) == 0 {
				continue
			}
			recordedByName := map[string]installEntry{}
			for _, e := range recorded {
				recordedByName[e.Name] = e
			}
			onDiskSet := map[string]bool{}
			for _, n := range onDisk {
				onDiskSet[n] = true
			}
			for _, name := range all {
				entry, hasEntry := recordedByName[name]
				repo, inRepo := byName[name]
				dest := ""
				payload := ""
				if inRepo {
					if repo.Binary != "" {
						dest = binaryDest(repo)
					}
					if len(repo.Files) > 0 {
						payload = serverDir(repo.Name)
					}
				}
				newer := false
				var entryPtr *installEntry
				if hasEntry {
					e := entry
					entryPtr = &e
				}
				if inRepo && hasEntry {
					newer = mcpIsNewer(repo, dest, payload, config, entryPtr)
				}
				var notes []string
				if hasEntry && !onDiskSet[name] {
					notes = append(notes, "not in config")
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
					var binary string
					if hasEntry && entry.BinaryPath != "" && isFile(entry.BinaryPath) {
						binary = entry.BinaryPath
					} else if dest != "" && isFile(dest) {
						binary = dest
					}
					if binary != "" {
						if mtime, ok := fileMtime(binary); ok {
							version = " - " + formatVersion(mtime)
						}
					}
					if newer && inRepo {
						repoVersion := formatVersion(mcpRepoMtime(repo))
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
