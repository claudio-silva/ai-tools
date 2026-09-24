package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Source is a repository aitools reads tools from: a local directory or a
// remote repo on a known host.
type Source struct {
	Raw       string // as typed by the user
	Kind      string // "local" or "remote"
	URL       string // normalized: abs path (local) or https://host/path (remote)
	Host      string // github | gitlab | bitbucket (remote only)
	RepoPath  string // owner/repo (or group/sub/repo on gitlab)
	LocalPath string // local only
}

type sourceMeta struct {
	URL       string `json:"url"`
	Ref       string `json:"ref,omitempty"`
	Commit    string `json:"commit,omitempty"`
	FetchedAt string `json:"fetchedAt"`
}

var hostPrefixes = map[string]string{
	"@gh/": "https://github.com/",
	"@bb/": "https://bitbucket.org/",
	"@gl/": "https://gitlab.com/",
}

var scpLike = regexp.MustCompile(`^git@([^:]+):(.+)$`)
var shorthand = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_./-]*$`)

// looksLikeSource reports whether a positional argument is a repo source
// rather than a tool name.
func looksLikeSource(arg string, bareDir bool) bool {
	if arg == "" {
		return false
	}
	for prefix := range hostPrefixes {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") ||
		strings.HasPrefix(arg, "ssh://") || scpLike.MatchString(arg) {
		return true
	}
	if strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "~") ||
		strings.HasPrefix(arg, ".") {
		return true
	}
	if shorthand.MatchString(arg) {
		return true
	}
	if bareDir && isDir(arg) {
		return true
	}
	return false
}

// parseSource normalizes a source argument into a Source.
func parseSource(arg string) *Source {
	src := &Source{Raw: arg}
	for prefix, base := range hostPrefixes {
		if strings.HasPrefix(arg, prefix) {
			return remoteSource(src, base+strings.TrimPrefix(arg, prefix))
		}
	}
	if m := scpLike.FindStringSubmatch(arg); m != nil {
		return remoteSource(src, "https://"+m[1]+"/"+m[2])
	}
	if strings.HasPrefix(arg, "ssh://") {
		if u, err := url.Parse(arg); err == nil && u.Host != "" {
			return remoteSource(src, "https://"+u.Host+strings.TrimSuffix(u.Path, ".git"))
		}
		die("%s: unsupported repo URL", arg)
	}
	if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
		return remoteSource(src, arg)
	}
	if strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "~") ||
		strings.HasPrefix(arg, ".") || isDir(arg) {
		path := resolve(arg)
		src.Kind = "local"
		src.LocalPath = path
		src.URL = path
		return src
	}
	if shorthand.MatchString(arg) {
		return remoteSource(src, "https://github.com/"+arg)
	}
	die("%s is not a repo source (expected a path, URL, or @gh/@bb/@gl prefix)", arg)
	return nil
}

func remoteSource(src *Source, rawURL string) *Source {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		die("%s: cannot parse repo URL", src.Raw)
	}
	host := strings.ToLower(u.Host)
	path := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
	parts := strings.Split(path, "/")
	var known string
	switch host {
	case "github.com":
		known = "github"
	case "gitlab.com":
		known = "gitlab"
	case "bitbucket.org":
		known = "bitbucket"
	default:
		die("%s: unsupported host %s (github.com, gitlab.com, bitbucket.org, or a local path)", src.Raw, u.Host)
	}
	if len(parts) < 2 {
		die("%s: expected an owner/repo URL", src.Raw)
	}
	// github/bitbucket repos are owner/repo; gitlab allows nested groups.
	if known != "gitlab" && len(parts) != 2 {
		die("%s: expected an owner/repo URL", src.Raw)
	}
	src.Kind = "remote"
	src.Host = known
	src.RepoPath = path
	src.URL = "https://" + host + "/" + path
	return src
}

func (s *Source) cacheKey() string {
	sum := sha1.Sum([]byte(s.URL))
	slug := strings.NewReplacer("/", "-", "_", "-").Replace(s.RepoPath)
	return slug + "-" + hex.EncodeToString(sum[:4])
}

func (s *Source) cacheDir() string {
	return filepath.Join(cacheRoot(), s.cacheKey())
}

// root returns the directory aitools reads the repo from: the local path, or
// the cached snapshot (fetching on first use).
func (s *Source) root() string {
	if s.Kind == "local" {
		if !isDir(s.LocalPath) {
			die("repo not found: %s", display(s.LocalPath))
		}
		return s.LocalPath
	}
	dir := s.cacheDir()
	if !isDir(dir) {
		fetchSnapshot(s)
	}
	return dir
}

// httpGet fetches a URL, dying with a friendly message on failure.
func httpGet(rawURL string) []byte {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		die("cannot fetch %s: %v", rawURL, err)
	}
	req.Header.Set("User-Agent", "aitools")
	resp, err := client.Do(req)
	if err != nil {
		die("cannot reach %s: %v", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusForbidden {
		die("%s: repo not found or private (only public repos are supported)", rawURL)
	}
	if resp.StatusCode != http.StatusOK {
		die("%s: HTTP %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		die("cannot read %s: %v", rawURL, err)
	}
	return body
}

func httpJSON(rawURL string) map[string]any {
	var data map[string]any
	if err := json.Unmarshal(httpGet(rawURL), &data); err != nil {
		die("%s: unexpected response", rawURL)
	}
	return data
}

// repoInfo resolves the default branch (and HEAD commit when cheap) for the
// repo's host.
func (s *Source) repoInfo() (branch, commit string) {
	encoded := url.PathEscape(s.RepoPath)
	switch s.Host {
	case "github":
		data := httpJSON("https://api.github.com/repos/" + s.RepoPath)
		branch, _ = data["default_branch"].(string)
		if branch != "" {
			c := httpJSON("https://api.github.com/repos/" + s.RepoPath + "/commits/" + branch)
			commit, _ = c["sha"].(string)
		}
	case "gitlab":
		data := httpJSON("https://gitlab.com/api/v4/projects/" + encoded)
		branch, _ = data["default_branch"].(string)
		if branch != "" {
			c := httpJSON("https://gitlab.com/api/v4/projects/" + encoded + "/repository/commits/" + url.PathEscape(branch))
			commit, _ = c["id"].(string)
		}
	case "bitbucket":
		data := httpJSON("https://api.bitbucket.org/2.0/repositories/" + s.RepoPath)
		if mb, ok := data["mainbranch"].(map[string]any); ok {
			branch, _ = mb["name"].(string)
		}
		if branch != "" {
			c := httpJSON("https://api.bitbucket.org/2.0/repositories/" + s.RepoPath + "/refs/branches/" + url.PathEscape(branch))
			if t, ok := c["target"].(map[string]any); ok {
				commit, _ = t["hash"].(string)
			}
		}
	}
	if branch == "" {
		die("%s: could not determine the default branch", s.URL)
	}
	return branch, commit
}

func (s *Source) tarballURL(branch string) string {
	switch s.Host {
	case "github":
		return "https://codeload.github.com/" + s.RepoPath + "/tar.gz/refs/heads/" + branch
	case "gitlab":
		repo := s.RepoPath[strings.LastIndex(s.RepoPath, "/")+1:]
		return "https://gitlab.com/" + s.RepoPath + "/-/archive/" + branch + "/" + repo + "-" + branch + ".tar.gz"
	case "bitbucket":
		return "https://bitbucket.org/" + s.RepoPath + "/get/" + branch + ".tar.gz"
	}
	die("unsupported host %s", s.Host)
	return ""
}

// fetchSnapshot downloads and extracts the repo tarball into the cache,
// replacing any existing snapshot atomically.
func fetchSnapshot(s *Source) {
	branch, commit := s.repoInfo()
	raw := httpGet(s.tarballURL(branch))
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		die("%s: invalid tarball", s.URL)
	}
	defer gz.Close()

	root := cacheRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		die("cannot create %s: %v", display(root), err)
	}
	tmp, err := os.MkdirTemp(root, ".fetch-")
	if err != nil {
		die("cannot create %s: %v", display(root), err)
	}
	defer os.RemoveAll(tmp)
	extractTarball(gz, tmp)

	final := s.cacheDir()
	os.RemoveAll(final)
	if err := os.Rename(tmp, final); err != nil {
		die("cannot store snapshot in %s: %v", display(final), err)
	}
	meta := sourceMeta{URL: s.URL, Ref: branch, Commit: commit, FetchedAt: time.Now().Format(time.RFC3339)}
	rawMeta, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(final, ".aitools-source.json"), append(rawMeta, '\n'), 0o644); err != nil {
		die("cannot write snapshot metadata: %v", err)
	}
}

// extractTarball writes tar entries into dir, stripping the top-level
// component every member shares. Guards against path escapes.
func extractTarball(r io.Reader, dir string) {
	tr := tar.NewReader(r)
	var sawEntries bool
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			die("cannot read repo tarball: %v", err)
		}
		sawEntries = true
		name := filepath.Clean(filepath.FromSlash(hdr.Name))
		parts := strings.Split(name, string(filepath.Separator))
		if len(parts) < 2 {
			continue // skip the top-level dir itself
		}
		rel := filepath.Join(parts[1:]...)
		dest := filepath.Join(dir, rel)
		if !isInside(dest, dir) && dest != dir {
			die("tarball member escapes the snapshot: %s", hdr.Name)
		}
		mode := os.FileMode(hdr.Mode & 0o777)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, mode|0o700); err != nil {
				die("cannot extract %s: %v", hdr.Name, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				die("cannot extract %s: %v", hdr.Name, err)
			}
			out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
			if err != nil {
				die("cannot extract %s: %v", hdr.Name, err)
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil || closeErr != nil {
				die("cannot extract %s", hdr.Name)
			}
			os.Chtimes(dest, time.Now(), hdr.ModTime)
		case tar.TypeSymlink:
			target := filepath.Clean(filepath.Join(filepath.Dir(dest), filepath.FromSlash(hdr.Linkname)))
			if !isInside(target, dir) {
				continue // ignore links escaping the snapshot
			}
			os.Remove(dest)
			os.Symlink(hdr.Linkname, dest)
		}
	}
	if !sawEntries {
		die("repo tarball is empty")
	}
}

func readSnapshotMeta(dir string) sourceMeta {
	var meta sourceMeta
	raw, err := os.ReadFile(filepath.Join(dir, ".aitools-source.json"))
	if err == nil {
		json.Unmarshal(raw, &meta)
	}
	return meta
}

func validateRepoDir(root, label string) {
	if !isDir(filepath.Join(root, "skills")) && !isDir(filepath.Join(root, "mcp")) {
		die("%s is not an aitools repository (no skills/ or mcp/ directory)", label)
	}
}

// --- config-backed default source -----------------------------------------

func defaultSource() *Source {
	raw := loadConfig().Source
	if raw == "" {
		return nil
	}
	return parseSource(raw)
}

// effectiveSource returns the repo positional > use default > nil.
func effectiveSource(repoArg string) *Source {
	if repoArg != "" {
		return parseSource(repoArg)
	}
	return defaultSource()
}

func requireSource(repoArg string) *Source {
	src := effectiveSource(repoArg)
	if src == nil {
		die("no repository selected; pass <repo> or run 'aitools use <repo>'")
	}
	return src
}

// --- commands --------------------------------------------------------------

func cmdUse(args *cliArgs) {
	if args.repo == "" {
		src := defaultSource()
		if src == nil {
			fmt.Println("no default source set; run 'aitools use <repo>' to set one")
		} else {
			fmt.Printf("default source: %s\n", src.URL)
		}
		return
	}
	if args.repo == "-" {
		cfg := loadConfig()
		if cfg.Source == "" {
			fmt.Println("no default source set")
			return
		}
		cfg.Source = ""
		saveConfig(cfg)
		fmt.Println("default source cleared")
		return
	}
	src := parseSource(args.repo)
	if src.Kind == "remote" {
		fetchSnapshot(src)
	}
	root := src.root()
	validateRepoDir(root, src.URL)
	cfg := loadConfig()
	cfg.Source = src.URL
	saveConfig(cfg)
	fmt.Printf("default source: %s\n", src.URL)
}

func cmdCache(args *cliArgs) {
	if len(args.names) > 0 && args.names[0] == "clear" {
		cmdCacheClear(args.names[1:])
		return
	}
	if len(args.names) > 0 {
		die("unknown cache action: %s (expected 'clear')", args.names[0])
	}
	root := cacheRoot()
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) == 0 {
		fmt.Println("no cached sources")
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		meta := readSnapshotMeta(dir)
		fmt.Println(meta.URL)
		detail := "    " + display(dir)
		if meta.FetchedAt != "" {
			if t, err := time.Parse(time.RFC3339, meta.FetchedAt); err == nil {
				detail += "  ·  fetched " + t.Format("2006-01-02 15:04")
			}
		}
		if meta.Ref != "" {
			detail += "  ·  ref " + meta.Ref
		}
		if meta.Commit != "" {
			detail += "  ·  " + shortCommit(meta.Commit)
		}
		if size := dirSize(dir); size > 0 {
			detail += "  ·  " + humanSize(size)
		}
		fmt.Println(detail)
	}
}

func cmdCacheClear(names []string) {
	root := cacheRoot()
	entries, _ := os.ReadDir(root)
	var targets []string
	if len(names) == 0 {
		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				targets = append(targets, filepath.Join(root, entry.Name()))
			}
		}
	} else {
		for _, name := range names {
			src := parseSource(name)
			if src.Kind != "remote" {
				die("%s: only remote repos are cached", name)
			}
			targets = append(targets, src.cacheDir())
		}
	}
	removed := 0
	for _, dir := range targets {
		if !isDir(dir) {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			die("cannot remove %s: %v", display(dir), err)
		}
		fmt.Printf("removed %s\n", display(dir))
		removed++
	}
	if removed == 0 {
		fmt.Println("cache is empty")
	}
}

func shortCommit(sha string) string {
	if len(sha) > 9 {
		return sha[:9]
	}
	return sha
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func dirSize(dir string) int64 {
	var total int64
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// --- pull ------------------------------------------------------------------

func gitCommit(repoDir string) string {
	git, err := exec.LookPath("git")
	if err != nil || !isDir(filepath.Join(repoDir, ".git")) {
		return ""
	}
	out, err := exec.Command(git, "-C", repoDir, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func cmdPull(args *cliArgs) int {
	src := requireSource(args.repo)
	if src.Kind == "remote" {
		fmt.Printf("repository  %s\n", src.URL)
		fetchSnapshot(src)
		meta := readSnapshotMeta(src.cacheDir())
		fmt.Printf("snapshot    %s\n", display(src.cacheDir()))
		if meta.Ref != "" {
			fmt.Printf("ref         %s\n", meta.Ref)
		}
		if meta.Commit != "" {
			fmt.Printf("commit      %s\n", shortCommit(meta.Commit))
		}
		return 0
	}
	root := src.LocalPath
	if !isDir(filepath.Join(root, ".git")) {
		die("%s is not a git clone, so there is no history to update", display(root))
	}
	git, err := exec.LookPath("git")
	if err != nil {
		die("git is required to update this repository")
	}
	before := gitCommit(root)
	fmt.Printf("repository  %s\n", display(root))
	if before != "" {
		fmt.Printf("commit      %s\n", before)
	}
	pull := exec.Command(git, "-C", root, "pull")
	pull.Stdin = os.Stdin
	pull.Stdout = os.Stdout
	pull.Stderr = os.Stderr
	code := 0
	if err := pull.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			die("git pull failed: %v", err)
		}
	}
	after := gitCommit(root)
	if code == 0 && after != "" && after != before {
		fmt.Printf("commit      %s\n", after)
	}
	return code
}
