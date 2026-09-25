package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const aboutURL = "https://github.com/claudio-silva/ai-tools/blob/main/ABOUT.md"

const helpText = `The tool manager for AI coding agents.

Tools are currently Skills and MCP servers.
List, install, update, remove or get information about tools, from any compatible Git repository.

Commands:
  use [<repo>]           Show or set the default source repository.
                         'use -' clears it.
  install [tool...]      Install tools. Defaults to --global.
  update [tool...]       Reinstall installed tools whose repository copy is
                         newer. No names, or --all, updates every outdated
                         install. Defaults to --global. Uninstalls, then
                         installs.
  uninstall [tool...]    Remove tools this command installed.
                         Defaults to --global.
  list [tool...]         List tools in the source repository, by platform.
  installed [tool...]    List tools installed on each platform.
                         Defaults to both global and local.
  about|info|show <tool> Show information about a tool, including install
                         status (◉ managed, ◎ other, ▲ update, or not
                         installed).
  pull                   Refresh the source repository snapshot (git pull for
                         local clones, re-download for remote repos).
  cache                  List cached repository snapshots.
  cache clear [<repo>]   Drop cached snapshots (all, or one repo).
  setup                  Installs or removes this command globally, for easy
                         access.
  help                   (or --help) Show this help.

Repository sources:
  The optional first argument selects the repository for that command;
  otherwise the 'use' default applies. A source is a local path, a URL, or
  a shorthand:
    owner/repo                     github.com/owner/repo
    @gh/owner/repo                 github.com
    @bb/owner/repo                 bitbucket.org
    @gl/group/repo                 gitlab.com
    https://github.com/owner/repo  full URLs
    ./path or /path                local directories
  Remote repos are downloaded once, cached, and refreshed by 'pull' or 'use'.

Options:
  -g, --global           User-level directories. Default for install, update,
                         and uninstall. Combine with --local to use both scopes.
                         For installed, show the user-level directories.
  -l, --local            Project directories. Combine with --global to use
                         both scopes. For installed, show the project.
  -C, --directory DIR    Project directory for --local. Default: current
                         directory.
  -p, --platform NAME    cursor, codex, claude-code, or devin. Repeat the flag,
                         or pass a comma-separated list.
  -a, --all              install: every matching tool in the repo. update:
                         every outdated install. uninstall: every recorded
                         install for the selected scope and platforms.
  -n, --dry-run          Show what install, update, or uninstall would change.
  --version              With installed, append each tool's version.
  --raw                  With install or update, write MCP $NAME placeholders
                         instead of filling them from the environment.
  -f, --force            install: replace a tool at the destination that was
                         not installed by this command. uninstall: also remove
                         copies that have no install record.
  -h, --help             Show this help.

setup options:
  --remove               Remove ~/bin/aitools or ~/.local/bin/aitools when it
                         is a copy of, or a link to, this executable.

Install locations:

  Skills
  Platform      Global                    Local
  cursor        ~/.cursor/skills          .cursor/skills
  codex         $CODEX_HOME/skills        .agents/skills
  claude-code   ~/.claude/skills          .claude/skills
  devin         ~/.config/devin/skills    .devin/skills

  MCP configs
  Platform      Global                         Local
  cursor        ~/.cursor/mcp.json             .cursor/mcp.json
  codex         $CODEX_HOME/config.toml        .codex/config.toml
  claude-code   ~/.claude.json                 .mcp.json
  devin         ~/.config/devin/mcp_config.json .devin/mcp_config.json

Notes:
  installed displays:
    ◎ for 3rd party tools,
    ◉ for tools managed by this installer,
    ▲ for managed tools that can be updated.
  Version numbers have the vYYMMDDHHmm format, which is derived from the tool
  files' modification time (whichever file is newer).
  $CODEX_HOME defaults to ~/.codex
  MCP installation supports scripting languages or macOS binaries.

Examples:
  aitools use @gh/claudio-silva/ai-toolbox
  aitools list
  aitools list @gh/owner/repo --platform cursor
  aitools show imagen
  aitools installed
  aitools installed --version
  aitools install --all
  aitools install imagen
  aitools install imagen --raw  #doesn't fill in API keys, etc.
  aitools install @gh/owner/repo cua-driver -p codex
  aitools uninstall auto-routing
  aitools update
  aitools update imagen
  aitools pull
  aitools cache clear
  aitools setup
  aitools setup --remove

The repository layout and manifest format are specified at ABOUT_URL
`

type cliArgs struct {
	command     string
	repo        string
	names       []string
	scopeGlobal bool
	local       bool
	directory   string
	platforms   []string
	all         bool
	dryRun      bool
	showVersion bool
	raw         bool
	remove      bool
	force       bool
}

func (a *cliArgs) with(names []string, all bool) *cliArgs {
	copy := *a
	copy.names = names
	copy.all = all
	return &copy
}

var boolLong = map[string]func(*cliArgs){
	"--global":  func(a *cliArgs) { a.scopeGlobal = true },
	"--local":   func(a *cliArgs) { a.local = true },
	"--all":     func(a *cliArgs) { a.all = true },
	"--dry-run": func(a *cliArgs) { a.dryRun = true },
	"--version": func(a *cliArgs) { a.showVersion = true },
	"--raw":     func(a *cliArgs) { a.raw = true },
	"--remove":  func(a *cliArgs) { a.remove = true },
	"--force":   func(a *cliArgs) { a.force = true },
	"--help":    func(a *cliArgs) {},
}
var valueLong = map[string]func(*cliArgs, string){
	"--directory": func(a *cliArgs, v string) { a.directory = v },
	"--platform":  func(a *cliArgs, v string) { a.platforms = append(a.platforms, v) },
}
var boolShort = map[byte]func(*cliArgs){
	'g': func(a *cliArgs) { a.scopeGlobal = true },
	'l': func(a *cliArgs) { a.local = true },
	'a': func(a *cliArgs) { a.all = true },
	'n': func(a *cliArgs) { a.dryRun = true },
	'f': func(a *cliArgs) { a.force = true },
	'h': func(a *cliArgs) {},
}
var valueShort = map[byte]func(*cliArgs, string){
	'C': func(a *cliArgs, v string) { a.directory = v },
	'p': func(a *cliArgs, v string) { a.platforms = append(a.platforms, v) },
}

// parseArgs parses command + flags + positionals; the first positional that
// looks like a repo source becomes args.repo.
func parseArgs(argv []string) *cliArgs {
	args := &cliArgs{}
	if len(argv) == 0 {
		return args
	}
	args.command = argv[0]
	var positional []string
	i := 1
	bareDirOK := args.command == "use"
	for ; i < len(argv); i++ {
		tok := argv[i]
		if tok == "--" {
			i++
			break
		}
		if tok == "-" || !strings.HasPrefix(tok, "-") {
			positional = append(positional, tok)
			continue
		}
		if strings.HasPrefix(tok, "--") {
			name, value, hasValue := strings.Cut(tok, "=")
			if fn, ok := boolLong[name]; ok {
				if hasValue {
					die("%s does not take a value", name)
				}
				fn(args)
				continue
			}
			if fn, ok := valueLong[name]; ok {
				if !hasValue {
					i++
					if i >= len(argv) {
						die("%s needs a value", name)
					}
					value = argv[i]
				}
				fn(args, value)
				continue
			}
			die("unknown option %s", name)
		}
		// short bundle; a value flag consumes the rest of the token or the next arg
		chars := tok[1:]
		for j := 0; j < len(chars); j++ {
			c := chars[j]
			if fn, ok := boolShort[c]; ok {
				fn(args)
				continue
			}
			if fn, ok := valueShort[c]; ok {
				value := chars[j+1:]
				if value == "" {
					i++
					if i >= len(argv) {
						die("-%c needs a value", c)
					}
					value = argv[i]
				}
				fn(args, value)
				break
			}
			die("unknown option -%c", c)
		}
	}
	for ; i < len(argv); i++ {
		positional = append(positional, argv[i])
	}
	// split repo from tool names
	if len(positional) > 0 && looksLikeSource(positional[0], bareDirOK) {
		args.repo = positional[0]
		positional = positional[1:]
	}
	args.names = positional
	return args
}

func wantsHelp(argv []string) bool {
	for _, arg := range argv {
		if arg == "--" {
			break
		}
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

var commandNames = map[string]bool{
	"use": true, "install": true, "update": true, "uninstall": true,
	"list": true, "installed": true, "about": true, "info": true, "show": true,
	"pull": true, "cache": true, "setup": true, "help": true,
}

func validate(args *cliArgs) {
	cmd := args.command
	switch cmd {
	case "use":
		// a single positional is always the repo for use
		if args.repo == "" && len(args.names) == 1 {
			args.repo = args.names[0]
			args.names = nil
		}
		if len(args.names) > 0 {
			die("use takes at most one repo")
		}
		return
	case "cache":
		return
	case "setup":
		if args.repo != "" || len(args.names) > 0 {
			die("setup does not take tool names")
		}
		return
	}
	if args.all && len(args.names) > 0 {
		die("pass tool names or --all, not both")
	}
	if (cmd == "install" || cmd == "uninstall") && !args.all && len(args.names) == 0 {
		die("name at least one tool, or pass --all")
	}
	switch cmd {
	case "list", "installed", "about", "pull":
		if args.all {
			die("--all does not apply to %s", cmd)
		}
		if args.dryRun {
			die("--dry-run does not apply to %s", cmd)
		}
	}
	if cmd == "list" && (args.scopeGlobal || args.local || args.directory != "") {
		die("list shows tools in the repository; use installed for installs")
	}
	if cmd == "about" {
		if len(args.names) != 1 {
			die("about, info, and show need one tool name")
		}
		if args.scopeGlobal || args.local || args.directory != "" || len(args.platforms) > 0 {
			die("about, info, and show list a repository tool and do not take install options")
		}
	}
	if cmd == "pull" && (len(args.names) > 0 || args.scopeGlobal || args.local ||
		args.directory != "" || len(args.platforms) > 0) {
		die("pull updates the source repository and does not take tool or install options")
	}
	if args.showVersion && cmd != "installed" {
		die("--version applies to installed")
	}
	if args.raw && cmd != "install" && cmd != "update" {
		die("--raw applies to install and update")
	}
	if args.force && cmd != "install" && cmd != "uninstall" {
		die("--force applies to install and uninstall")
	}
}

// probeForeign checks whether name has an unrecorded presence: a skill dir
// or an MCP config entry under the selected scopes/platforms.
func probeForeign(name string, args *cliArgs) (skill, mcp bool) {
	scopes := selectedScopes(args, "global")
	platformFilter := selectedPlatforms(args.platforms)
	project := ""
	if contains(scopes, "local") {
		project = projectDir(args)
	}
	platforms := platformFilter
	if platforms == nil {
		platforms = platformOrder
	}
	for _, scope := range scopes {
		p := ""
		if scope == "local" {
			p = project
		}
		for _, platform := range platforms {
			skillsDir, _ := platformPaths(platform, scope, p)
			if present(filepath.Join(skillsDir, name)) {
				skill = true
			}
			if configHasServer(mcpConfigPath(platform, scope, p), name) {
				mcp = true
			}
		}
	}
	return skill, mcp
}

// partitionTools splits names into skill and MCP names for a catalog.
func partitionTools(args *cliArgs, cat *catalog, data *stateData, receipts bool) (skillNames, mcpNames []string) {
	names := args.names
	repoSkills := map[string]bool{}
	repoMcps := map[string]bool{}
	if cat != nil {
		for _, s := range cat.skills {
			repoSkills[s.Name] = true
		}
		for _, m := range cat.mcps {
			repoMcps[m.Name] = true
		}
	}
	knownSkills := map[string]bool{}
	knownMcps := map[string]bool{}
	for k, v := range repoSkills {
		knownSkills[k] = v
	}
	for k, v := range repoMcps {
		knownMcps[k] = v
	}
	if receipts {
		for i := range data.Installs {
			e := &data.Installs[i]
			if e.kind() == "mcp" && e.Name != "" {
				knownMcps[e.Name] = true
			} else if e.kind() == "skill" && e.Skill != "" {
				knownSkills[e.Skill] = true
			}
		}
	}
	var unknown, both []string
	for _, name := range names {
		inSkill, inMcp := knownSkills[name], knownMcps[name]
		switch {
		case inSkill && inMcp:
			both = append(both, name)
		case inSkill:
			skillNames = append(skillNames, name)
		case inMcp:
			mcpNames = append(mcpNames, name)
		case args.force && args.command == "uninstall":
			// foreign name: probe for an unrecorded presence
			hasSkill, hasMcp := probeForeign(name, args)
			if hasSkill {
				skillNames = append(skillNames, name)
			}
			if hasMcp {
				mcpNames = append(mcpNames, name)
			}
			if !hasSkill && !hasMcp {
				fmt.Printf("%s is not installed\n", name)
			}
		default:
			unknown = append(unknown, name)
		}
	}
	if len(both) > 0 {
		die("%s matches both a skill and an MCP server", strings.Join(both, ", "))
	}
	if len(unknown) > 0 {
		die("unknown tool: %s", strings.Join(unknown, ", "))
	}
	return skillNames, mcpNames
}

func cmdListTools(args *cliArgs, cat *catalog, nameFilter map[string]bool) {
	platformFilter := selectedPlatforms(args.platforms)
	platforms := platformFilter
	if platforms == nil {
		platforms = platformOrder
	}
	for _, platform := range platforms {
		fmt.Println(platform)
		var rows []string
		for _, skill := range cat.skills {
			if contains(skill.Platforms, platform) && (len(nameFilter) == 0 || nameFilter[skill.Name]) {
				rows = append(rows, skill.Name)
			}
		}
		for _, server := range cat.mcps {
			if contains(server.Platforms, platform) && (len(nameFilter) == 0 || nameFilter[server.Name]) {
				rows = append(rows, server.Name+"  (mcp)")
			}
		}
		sort.Slice(rows, func(i, j int) bool {
			return strings.Fields(rows[i])[0] < strings.Fields(rows[j])[0]
		})
		if len(rows) == 0 {
			fmt.Println("  (none)")
		} else {
			for _, row := range rows {
				fmt.Printf("  %s\n", row)
			}
		}
		fmt.Println()
	}
}

// mergedCatalog builds one lookup view over every relevant repository: each
// recorded source first (a tool's origin repo wins for it), then the effective
// source for names nothing else provides. Each tool keeps its own Root and
// Source, so manifests and mtimes resolve against the right snapshot.
func mergedCatalog(repoArg string, data *stateData) *catalog {
	merged := &catalog{}
	seenSkill := map[string]bool{}
	seenMcp := map[string]bool{}
	add := func(cat *catalog, only map[string]bool) {
		if cat == nil {
			return
		}
		for _, s := range cat.skills {
			if seenSkill[s.Name] || (only != nil && !only[s.Name]) {
				continue
			}
			seenSkill[s.Name] = true
			merged.skills = append(merged.skills, s)
		}
		for _, srv := range cat.mcps {
			if seenMcp[srv.Name] || (only != nil && !only[srv.Name]) {
				continue
			}
			seenMcp[srv.Name] = true
			merged.mcps = append(merged.mcps, srv)
		}
	}
	// an explicit repo argument always takes precedence
	explicitURL := ""
	if repoArg != "" {
		src := parseSource(repoArg)
		add(catalogFor(src), nil)
		explicitURL = src.URL
	}
	seenSources := map[string]bool{}
	for i := range data.Installs {
		e := &data.Installs[i]
		if e.Source == "" || e.Source == explicitURL || seenSources[e.Source] {
			continue
		}
		seenSources[e.Source] = true
		cat := tryCatalog(parseSource(e.Source))
		if cat == nil {
			continue
		}
		// only the tools actually recorded from this source
		names := map[string]bool{}
		for j := range data.Installs {
			oe := &data.Installs[j]
			if oe.Source == e.Source {
				if oe.kind() == "skill" {
					names[oe.Skill] = true
				} else {
					names[oe.Name] = true
				}
			}
		}
		add(cat, names)
	}
	if src := effectiveSource(repoArg); src != nil {
		add(tryCatalog(src), nil)
	}
	return merged
}

func findSkill(cat *catalog, name string) (Skill, bool) {
	for _, s := range cat.skills {
		if s.Name == name {
			return s, true
		}
	}
	return Skill{}, false
}

func findMcp(cat *catalog, name string) (*McpServer, bool) {
	for _, s := range cat.mcps {
		if s.Name == name {
			return s, true
		}
	}
	return nil, false
}

func tryCatalog(src *Source) (cat *catalog) {
	defer func() {
		if r := recover(); r != nil {
			if ue, ok := r.(*userError); ok {
				fmt.Fprintf(os.Stderr, "warning: %s\n", ue.msg)
				cat = nil
				return
			}
			panic(r)
		}
	}()
	return catalogFor(src)
}

func selectedScopes(args *cliArgs, def string) []string {
	var chosen []string
	if args.scopeGlobal {
		chosen = append(chosen, "global")
	}
	if args.local {
		chosen = append(chosen, "local")
	}
	if len(chosen) == 0 {
		return []string{def}
	}
	return chosen
}

func selectedPlatforms(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	var chosen []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			name := strings.TrimSpace(part)
			if name == "" {
				continue
			}
			if !contains(platformOrder, name) {
				die("unknown platform %q; expected %s", name, strings.Join(platformOrder, ", "))
			}
			if !contains(chosen, name) {
				chosen = append(chosen, name)
			}
		}
	}
	return chosen
}

func projectDir(args *cliArgs) string {
	raw := args.directory
	if raw == "" {
		raw = cwd()
	}
	path := resolve(raw)
	if !isDir(path) {
		die("project directory does not exist: %s", path)
	}
	return path
}

func installedScopes(args *cliArgs) ([]string, string) {
	var scopes []string
	if args.scopeGlobal {
		scopes = append(scopes, "global")
	}
	if args.local {
		scopes = append(scopes, "local")
	}
	if len(scopes) == 0 {
		scopes = []string{"global", "local"}
	}
	if args.directory != "" && !contains(scopes, "local") {
		die("--directory is used with --local")
	}
	project := ""
	if contains(scopes, "local") {
		project = projectDir(args)
	}
	return scopes, project
}

// --- help --------------------------------------------------------------------

var helpHeadings = map[string]bool{
	"Commands:": true, "Options:": true, "setup options:": true,
	"Install locations:": true, "Notes:": true, "Examples:": true,
	"Repository sources:": true,
}

var (
	reUsage   = regexp.MustCompile(`^(Usage: |usage: )(\S+)(.*)$`)
	reCommand = regexp.MustCompile(`^(  )([a-z][\w-]*)(.*)$`)
	reOption  = regexp.MustCompile(`^(  )(-.*)(\s{2,}.*)$`)
	reExample = regexp.MustCompile(`^(  )(aitools)(.*)$`)
)

func colorizeHelp(text string) string {
	if !colorEnabled() {
		return text
	}
	// palette from Python's argparse theme
	const (
		usage, prog, progExtra = "1;34", "1;35", "35"
		heading, action        = "1;34", "1;32"
		longOpt, shortOpt      = "1;36", "1;32"
		summaryAction, reset   = "32", "0"
	)
	st := func(code, s string) string { return "\x1b[" + code + "m" + s + "\x1b[0m" }
	colorFlags := func(s string) string {
		var parts []string
		for _, token := range strings.Split(s, ", ") {
			if strings.HasPrefix(token, "--") {
				parts = append(parts, st(longOpt, token))
			} else if strings.HasPrefix(token, "-") {
				parts = append(parts, st(shortOpt, token))
			} else {
				parts = append(parts, token)
			}
		}
		return strings.Join(parts, ", ")
	}
	var out strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		body := strings.TrimSuffix(line, "\n")
		nl := ""
		if body != line {
			nl = "\n"
		}
		if m := reUsage.FindStringSubmatch(body); m != nil {
			out.WriteString(st(usage, m[1]) + st(prog, m[2]) + st(progExtra, m[3]) + nl)
			continue
		}
		if helpHeadings[body] {
			out.WriteString(st(heading, body) + nl)
			continue
		}
		if m := reCommand.FindStringSubmatch(body); m != nil {
			if commandNames[m[2]] {
				out.WriteString(m[1] + st(action, m[2]) + m[3] + nl)
				continue
			}
		}
		if m := reOption.FindStringSubmatch(body); m != nil {
			out.WriteString(m[1] + colorFlags(m[2]) + m[3] + nl)
			continue
		}
		if m := reExample.FindStringSubmatch(body); m != nil {
			out.WriteString(m[1] + st(prog, m[2]) + m[3] + nl)
			continue
		}
		const aboutPrefix = "The repository layout and manifest format are specified at "
		if strings.HasPrefix(body, aboutPrefix) {
			path := strings.TrimSuffix(strings.TrimPrefix(body, aboutPrefix), ".")
			period := ""
			if strings.HasSuffix(body, ".") {
				period = "."
			}
			out.WriteString(aboutPrefix + "\x1b[34m" + path + "\x1b[0m" + period + nl)
			continue
		}
		if strings.HasPrefix(body, "installed displays:") {
			out.WriteString(st(summaryAction, "installed") + body[len("installed"):] + nl)
			continue
		}
		if body == "  Skills" || body == "  MCP configs" {
			out.WriteString(st(heading, body) + nl)
			continue
		}
		if matched, _ := regexp.MatchString(`^  Platform\s+Global\s+Local`, body); matched {
			out.WriteString("\x1b[1m" + body + "\x1b[0m" + nl)
			continue
		}
		out.WriteString(line)
	}
	return out.String()
}

func showHelp() {
	fmt.Println("usage: aitools <command> [options] [tool...]")
	fmt.Println()
	fmt.Print(colorizeHelp(strings.ReplaceAll(helpText, "ABOUT_URL", aboutURL)))
}

// --- setup -------------------------------------------------------------------

func pathDirOnPath(dir string) bool {
	resolved := resolve(dir)
	for _, entry := range strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)) {
		if entry == "" {
			continue
		}
		if resolve(entry) == resolved {
			return true
		}
	}
	return false
}

func cmdSetup(remove bool) {
	exe := selfExe()
	var links []string
	for _, dir := range setupDirs() {
		links = append(links, dir+"/aitools")
	}
	if remove {
		removed := false
		for _, link := range links {
			isOurs := false
			if isSymlink(link) && resolve(link) == exe {
				isOurs = true
			} else if isFile(link) && sameFileContent(link, exe) {
				isOurs = true
			}
			if !isOurs {
				continue
			}
			os.Remove(link)
			fmt.Printf("removed %s\n", displayLink(link))
			removed = true
		}
		if !removed {
			die("no aitools copy or link in %s or %s", displayLink(links[0]), displayLink(links[1]))
		}
		return
	}
	// already installed?
	for _, link := range links {
		if isSymlink(link) && resolve(link) == exe {
			fmt.Printf("already set up: %s -> %s\n", displayLink(link), display(exe))
			return
		}
		if isFile(link) && sameFileContent(link, exe) {
			fmt.Printf("already set up: %s is this executable\n", displayLink(link))
			return
		}
	}
	var available []string
	for _, dir := range setupDirs() {
		if pathDirOnPath(dir) {
			available = append(available, dir)
		}
	}
	if len(available) == 0 {
		die("~/bin and ~/.local/bin are not on PATH. Create one of them, add it to PATH, and run setup again.")
	}
	var skipped []string
	for _, dir := range available {
		link := filepath.Join(dir, "aitools")
		if isSymlink(link) {
			target := resolve(link)
			if target == exe {
				fmt.Printf("already linked: %s -> %s\n", displayLink(link), display(exe))
				return
			}
			skipped = append(skipped, fmt.Sprintf("%s links to %s", displayLink(link), display(target)))
			continue
		}
		if exists(link) {
			skipped = append(skipped, fmt.Sprintf("%s exists and is not a symlink", displayLink(link)))
			continue
		}
		if !isDir(dir) {
			if exists(dir) {
				skipped = append(skipped, fmt.Sprintf("%s exists and is not a directory", displayLink(dir)))
				continue
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				skipped = append(skipped, fmt.Sprintf("could not create %s: %v", displayLink(dir), err))
				continue
			}
		}
		if !writable(dir) {
			skipped = append(skipped, fmt.Sprintf("%s is not writable", displayLink(dir)))
			continue
		}
		copyFile(exe, link)
		os.Chmod(link, 0o755)
		fmt.Printf("installed %s -> %s\n", displayLink(link), display(exe))
		warnIfShadowed(link)
		offerRepoRemoval(exe)
		return
	}
	die("could not install aitools: %s", strings.Join(skipped, "; "))
}

func sameFileContent(a, b string) bool {
	da, err1 := os.ReadFile(a)
	db, err2 := os.ReadFile(b)
	return err1 == nil && err2 == nil && string(da) == string(db)
}

func warnIfShadowed(link string) {
	found, err := exec.LookPath("aitools")
	if err != nil || resolve(found) == resolve(link) {
		return
	}
	fmt.Printf("note: aitools on PATH is %s, so %s is hidden\n", displayLink(found), displayLink(link))
}

// offerRepoRemoval suggests dropping the checkout the binary was run from,
// since the installed copy no longer needs it.
func offerRepoRemoval(exe string) {
	root := gitRootOf(exe)
	if root == "" || !isTTY(os.Stdin) || !isTTY(os.Stdout) {
		return
	}
	fmt.Printf("\nThe installed binary is self-contained. Remove the checkout at %s? [y/N] ", display(root))
	answer := readLine(bufio.NewReader(os.Stdin))
	if strings.EqualFold(strings.TrimSpace(answer), "y") {
		trashPath(root)
		fmt.Printf("removed %s\n", display(root))
	}
}

// gitRootOf returns the git worktree root containing path, or "".
func gitRootOf(path string) string {
	dir := filepath.Dir(path)
	for {
		if exists(dir + "/.git") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// --- main ---------------------------------------------------------------------

func main() {
	code := run(os.Args[1:])
	os.Exit(code)
}

func run(argv []string) (code int) {
	defer func() {
		if r := recover(); r != nil {
			if ue, ok := r.(*userError); ok {
				fmt.Fprintf(os.Stderr, "error: %s\n", ue.msg)
				code = 1
				return
			}
			panic(r)
		}
	}()
	if wantsHelp(argv) {
		showHelp()
		return 0
	}
	args := parseArgs(argv)
	if args.command == "" {
		fmt.Println("usage: aitools <command> [options] [tool...]")
		return 1
	}
	if args.command == "help" {
		showHelp()
		return 0
	}
	if !commandNames[args.command] {
		die("unknown command %s", args.command)
	}
	if args.command == "info" || args.command == "show" {
		args.command = "about"
	}
	validate(args)
	switch args.command {
	case "use":
		cmdUse(args)
		return 0
	case "cache":
		cmdCache(args)
		return 0
	case "setup":
		cmdSetup(args.remove)
		return 0
	case "pull":
		return cmdPull(args)
	}
	data := loadState()
	receipts := args.command == "uninstall" || args.command == "installed"

	switch args.command {
	case "install":
		src := requireSource(args.repo)
		cat := catalogFor(src)
		skillNames, mcpNames := partitionTools(args, cat, data, false)
		if args.raw && !args.all && len(mcpNames) == 0 {
			die("--raw applies to MCP servers")
		}
		if args.all || len(skillNames) > 0 {
			skillCmdInstall(args.with(skillNames, args.all), cat, skillNames, data, args.all)
		}
		if args.all || len(mcpNames) > 0 {
			mcpCmdInstall(args.with(mcpNames, args.all), cat, mcpNames, data, args.all)
		}
	case "list":
		cat := catalogFor(requireSource(args.repo))
		nameFilter := map[string]bool{}
		for _, n := range args.names {
			nameFilter[n] = true
		}
		cmdListTools(args, cat, nameFilter)
	case "about":
		cat := catalogFor(requireSource(args.repo))
		name := args.names[0]
		if _, ok := findSkill(cat, name); ok {
			skillCmdAbout(args, cat, name, data)
		} else if _, ok := findMcp(cat, name); ok {
			mcpCmdAbout(args, cat, name, data)
		} else {
			die("unknown tool: %s", name)
		}
	case "installed":
		cat := mergedCatalog(args.repo, data)
		scopes, project := installedScopes(args)
		skillNames, mcpNames := partitionTools(args, cat, data, receipts)
		var sections []struct {
			Title  string
			Groups []installedGroup
		}
		sections = append(sections, struct {
			Title  string
			Groups []installedGroup
		}{"Skills", collectInstalledSkills(args, cat, skillNames, data, scopes, project)})
		sections = append(sections, struct {
			Title  string
			Groups []installedGroup
		}{"MCP servers", collectInstalledMcps(args, cat, mcpNames, data, scopes, project)})
		renderInstalled(sections, scopes, project)
	case "uninstall":
		cat := mergedCatalog(args.repo, data)
		skillNames, mcpNames := partitionTools(args, cat, data, receipts)
		if args.all || len(skillNames) > 0 {
			skillCmdUninstall(args.with(skillNames, args.all), cat, skillNames, data, args.all)
		}
		if args.all || len(mcpNames) > 0 {
			mcpCmdUninstall(args.with(mcpNames, args.all), cat, mcpNames, data, args.all)
		}
	case "update":
		var cat *catalog
		if args.repo != "" {
			cat = catalogFor(parseSource(args.repo))
		} else {
			cat = mergedCatalog("", data)
		}
		skillNames, mcpNames := partitionTools(args, cat, data, receipts)
		changed := false
		if len(skillNames) > 0 || (len(args.names) == 0 && len(cat.skills) > 0) {
			changed = skillCmdUpdate(args.with(skillNames, len(args.names) == 0), cat, skillNames, data, len(args.names) == 0) || changed
		}
		if len(mcpNames) > 0 || (len(args.names) == 0 && len(cat.mcps) > 0) {
			changed = mcpCmdUpdate(args.with(mcpNames, len(args.names) == 0), cat, mcpNames, data, len(args.names) == 0) || changed
		}
		if !changed && len(args.names) == 0 {
			fmt.Println("nothing to update")
		}
	default:
		die("unknown command %s", args.command)
	}
	return 0
}
