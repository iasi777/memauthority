// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package publiccli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iasi777/v-memory/internal/mcpserver"
	"github.com/iasi777/v-memory/internal/oauthserver"
	"github.com/iasi777/v-memory/internal/pathguard"
	"github.com/iasi777/v-memory/internal/vaultvalidate"
)

const (
	serviceGitName  = "V-Memory MCP"
	serviceGitEmail = "v-memory@v-memory.invalid"
)

var (
	initialIndex      = []byte("schema_version: 4\nprojects: {}\n")
	initialAttributes = []byte("*.md text eol=lf\n*.yaml text eol=lf\n")
)

// Run executes the public v-memory CLI and returns a process-style exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printRootHelp(stdout)
		return 0
	}
	if args[0] == "--version" {
		if len(args) != 1 {
			fmt.Fprintln(stderr, "Usage: v-memory --version")
			return 2
		}
		printVersion(stdout)
		return 0
	}
	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printRootHelp(stderr)
		return 2
	}
}

func printRootHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  v-memory init <path>")
	fmt.Fprintln(w, "  v-memory validate [--json] [path]")
	fmt.Fprintln(w, "  v-memory serve --vault <path> --state-dir <path> [options]")
	fmt.Fprintln(w, "  v-memory version")
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "v-memory %s\n", mcpserver.ImplementationVersion)
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "Usage: v-memory version")
		return 2
	}
	printVersion(stdout)
	return 0
}

func runInit(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(stdout, "Usage: v-memory init <path>")
		return 0
	}
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(stderr, "Usage: v-memory init <path>")
		return 2
	}
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(stderr, "git executable not found")
		return 2
	}
	target, err := filepath.Abs(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "cannot resolve target path")
		return 2
	}
	target = filepath.Clean(target)

	if info, statErr := os.Lstat(target); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			fmt.Fprintln(stderr, "init target must be a real directory path")
			return 1
		}
		if initializedVault(target) {
			fmt.Fprintln(stdout, "V-Memory Vault already initialized")
			return 0
		}
		entries, readErr := os.ReadDir(target)
		if readErr != nil {
			fmt.Fprintln(stderr, "cannot inspect init target")
			return 2
		}
		if len(entries) != 0 {
			fmt.Fprintln(stderr, "refusing to initialize non-empty unknown directory")
			return 1
		}
	} else if !os.IsNotExist(statErr) {
		fmt.Fprintln(stderr, "cannot inspect init target")
		return 2
	}

	nested, nestedErr := insideExistingGitWorktree(target)
	if nestedErr != nil {
		fmt.Fprintln(stderr, nestedErr.Error())
		return 2
	}
	if nested {
		fmt.Fprintln(stderr, "refusing to initialize a nested Git repository")
		return 1
	}

	existed := pathExists(target)
	if err := os.MkdirAll(target, 0o755); err != nil {
		fmt.Fprintln(stderr, "cannot create init target")
		return 2
	}
	success := false
	defer func() {
		if success {
			return
		}
		if !existed {
			_ = os.RemoveAll(target)
			return
		}
		_ = os.RemoveAll(filepath.Join(target, ".git"))
		_ = os.Remove(filepath.Join(target, "INDEX.yaml"))
		_ = os.Remove(filepath.Join(target, ".gitattributes"))
	}()

	if _, err := runGit(target, nil, "init", "-q", "-b", "main"); err != nil {
		fmt.Fprintln(stderr, "git init failed")
		return 2
	}
	if err := os.WriteFile(filepath.Join(target, "INDEX.yaml"), initialIndex, 0o644); err != nil {
		fmt.Fprintln(stderr, "write INDEX.yaml failed")
		return 2
	}
	if err := os.WriteFile(filepath.Join(target, ".gitattributes"), initialAttributes, 0o644); err != nil {
		fmt.Fprintln(stderr, "write .gitattributes failed")
		return 2
	}
	report := vaultvalidate.Validate(target)
	if !report.Valid {
		fmt.Fprintln(stderr, "generated Vault failed validation")
		return 2
	}
	if _, err := runGit(target, nil, "add", "--", "INDEX.yaml", ".gitattributes"); err != nil {
		fmt.Fprintln(stderr, "git add failed")
		return 2
	}
	tree, err := runGit(target, nil, "write-tree")
	if err != nil {
		fmt.Fprintln(stderr, "git write-tree failed")
		return 2
	}
	commitEnv := append(gitEnvironment(),
		"GIT_AUTHOR_NAME="+serviceGitName,
		"GIT_AUTHOR_EMAIL="+serviceGitEmail,
		"GIT_COMMITTER_NAME="+serviceGitName,
		"GIT_COMMITTER_EMAIL="+serviceGitEmail,
	)
	commit, err := runGit(target, commitEnv, "commit-tree", tree, "-m", "v-memory: initialize empty Vault")
	if err != nil {
		fmt.Fprintln(stderr, "git commit-tree failed")
		return 2
	}
	if _, err := runGit(target, nil, "update-ref", "refs/heads/main", commit); err != nil {
		fmt.Fprintln(stderr, "git update-ref failed")
		return 2
	}
	clean, err := runGit(target, nil, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil || strings.TrimSpace(clean) != "" {
		fmt.Fprintln(stderr, "initialized Vault is not clean")
		return 2
	}
	success = true
	fmt.Fprintln(stdout, "Initialized empty V-Memory Vault")
	return 0
}

func runValidate(args []string, stdout, stderr io.Writer) int {
	jsonOutput := false
	target := "."
	positional := 0
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			fmt.Fprintln(stdout, "Usage: v-memory validate [--json] [path]")
			return 0
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "unsupported validate option %q\n", arg)
				return 2
			}
			positional++
			if positional > 1 {
				fmt.Fprintln(stderr, "Usage: v-memory validate [--json] [path]")
				return 2
			}
			target = arg
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(stderr, "git executable not found")
		return 2
	}

	report := vaultvalidate.Validate(target)
	gitFindings, executionErr := validateGitEnvelope(target)
	if executionErr != nil {
		fmt.Fprintln(stderr, executionErr.Error())
		return 2
	}
	report.Errors = append(report.Errors, gitFindings...)
	sortFindings(report.Errors)
	report.Valid = len(report.Errors) == 0

	if jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(report); err != nil {
			fmt.Fprintln(stderr, "write validation JSON failed")
			return 2
		}
	} else if report.Valid {
		fmt.Fprintln(stdout, "VALID")
	} else {
		fmt.Fprintf(stdout, "INVALID: %d error(s)\n", len(report.Errors))
		for _, f := range report.Errors {
			loc := f.Path
			if f.Line > 0 {
				loc += fmt.Sprintf(":%d", f.Line)
			}
			if loc == "" {
				loc = "-"
			}
			field := ""
			if f.Field != "" {
				field = " field=" + f.Field
			}
			fmt.Fprintf(stdout, "ERROR %s %s%s: %s\n", f.Code, loc, field, f.Message)
		}
	}
	if report.Valid {
		return 0
	}
	return 1
}

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	vault := fs.String("vault", "", "Vault Git root")
	stateDir := fs.String("state-dir", "", "runtime state directory outside Vault Authority")
	transport := fs.String("transport", "stdio", "stdio or http")
	listen := fs.String("listen", "127.0.0.1:8000", "HTTP listen address")
	oauthEnabled := fs.Bool("oauth-enabled", true, "protect HTTP transport with embedded OAuth; ignored by stdio")
	publicBase := fs.String("public-base-url", os.Getenv("VM_OAUTH_ISSUER_URL"), "public OAuth issuer origin for HTTP mode")
	oauthClientID := fs.String("oauth-client-id", os.Getenv("VM_OAUTH_CLIENT_ID"), "pre-registered OAuth client id")
	oauthClientSecretFile := fs.String("oauth-client-secret-file", os.Getenv("VM_OAUTH_CLIENT_SECRET_FILE"), "optional OAuth confidential-client secret file")
	oauthRedirectURIs := fs.String("oauth-redirect-uris", os.Getenv("VM_OAUTH_REDIRECT_URIS"), "comma-separated exact redirect URIs")
	oauthUsername := fs.String("oauth-username", os.Getenv("VM_OAUTH_USERNAME"), "local OAuth login username")
	oauthPasswordFile := fs.String("oauth-password-file", os.Getenv("VM_OAUTH_PASSWORD_FILE"), "local OAuth login password file")
	oauthStateDB := fs.String("oauth-state-db", os.Getenv("VM_OAUTH_STATE_DB"), "OAuth state SQLite path outside Vault")
	staticTokenFile := fs.String("auth-token-file", os.Getenv("VM_AUTH_TOKEN_FILE"), "optional deployment Bearer token file accepted by protected MCP")
	allowedHosts := fs.String("allowed-hosts", os.Getenv("VM_ALLOWED_HOSTS"), "comma-separated exact HTTP Host allowlist")
	allowedOrigins := fs.String("allowed-origins", os.Getenv("VM_ALLOWED_ORIGINS"), "comma-separated exact HTTP Origin allowlist")
	writeEnabled := fs.Bool("write-enabled", false, "enable Authority mutation tools; disabled by default")
	writeSource := fs.String("write-source", os.Getenv("VM_WRITE_SOURCE"), "mutation source label recorded in structured results")
	requirePrimary := fs.Bool("require-primary", os.Getenv("VM_REQUIRE_PRIMARY") == "1", "require Git-tracked PRIMARY node/epoch match before mutation")
	primaryEpoch := fs.Int("primary-epoch", 0, "expected primary epoch when fencing is required")
	nodeIDFile := fs.String("node-id-file", os.Getenv("VM_NODE_ID_FILE"), "external local node-id file used for primary fencing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(*vault) == "" || strings.TrimSpace(*stateDir) == "" {
		fmt.Fprintln(stderr, "Usage: v-memory serve --vault <path> --state-dir <path> [options]")
		return 2
	}
	if *transport != "stdio" && *transport != "http" {
		fmt.Fprintln(stderr, "--transport must be stdio or http")
		return 2
	}
	if *transport == "http" && *writeEnabled && !*oauthEnabled {
		fmt.Fprintln(stderr, "write-enabled HTTP requires OAuth")
		return 2
	}
	if *transport == "http" {
		if err := validateHTTPExposure(*listen, *oauthEnabled); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	vaultRoot, err := filepath.Abs(*vault)
	if err != nil {
		fmt.Fprintln(stderr, "cannot resolve Vault path")
		return 2
	}
	stateRoot, err := filepath.Abs(*stateDir)
	if err != nil {
		fmt.Fprintln(stderr, "cannot resolve state directory")
		return 2
	}
	inside, containErr := pathWithinCanonical(vaultRoot, stateRoot)
	if containErr != nil {
		fmt.Fprintln(stderr, "cannot resolve state directory containment")
		return 2
	}
	if inside {
		fmt.Fprintln(stderr, "state directory must be outside Vault Authority")
		return 1
	}
	if code, message := serveAdmission(vaultRoot); code != 0 {
		fmt.Fprintln(stderr, message)
		return code
	}

	options := mcpserver.OpenOptions{WriteEnabled: *writeEnabled, WriteSource: *writeSource, ManagedAdmission: true, AllowedHosts: splitCSV(*allowedHosts), AllowedOrigins: splitCSV(*allowedOrigins)}
	if *requirePrimary {
		if *primaryEpoch < 1 || strings.TrimSpace(*nodeIDFile) == "" {
			fmt.Fprintln(stderr, "--require-primary requires --primary-epoch >= 1 and --node-id-file")
			return 2
		}
		raw, err := os.ReadFile(*nodeIDFile)
		if err != nil || strings.TrimSpace(string(raw)) == "" {
			fmt.Fprintln(stderr, "read non-empty --node-id-file failed")
			return 2
		}
		options.RequirePrimary = true
		options.PrimaryEpoch = *primaryEpoch
		options.LocalNodeID = strings.TrimSpace(string(raw))
	}
	if *transport == "http" && *oauthEnabled {
		stateDB := *oauthStateDB
		if stateDB == "" {
			stateDB = filepath.Join(stateRoot, "oauth.sqlite3")
		}
		redirects := splitCSV(*oauthRedirectURIs)
		if *publicBase == "" || *oauthClientID == "" || len(redirects) == 0 || *oauthUsername == "" || *oauthPasswordFile == "" {
			fmt.Fprintln(stderr, "HTTP OAuth requires --public-base-url, --oauth-client-id, --oauth-redirect-uris, --oauth-username, and --oauth-password-file")
			return 2
		}
		options.OAuth = &oauthserver.Config{Issuer: *publicBase, ClientID: *oauthClientID, RedirectURIs: redirects, Username: *oauthUsername, PasswordFile: *oauthPasswordFile, ClientSecretFile: *oauthClientSecretFile, StaticTokenFile: *staticTokenFile, StateDB: stateDB}
	}
	app, err := mcpserver.OpenWithOptions(vaultRoot, stateRoot, options)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer app.Close()
	switch *transport {
	case "stdio":
		if err := app.RunStdio(context.Background()); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	case "http":
		server := newHTTPServer(*listen, app.HTTPHandler())
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(stderr, err)
			return 2
		}
		return 0
	}
	return 2
}

func serveAdmission(root string) (int, string) {
	if _, err := exec.LookPath("git"); err != nil {
		return 2, "git executable not found"
	}
	report := vaultvalidate.Validate(root)
	findings, executionErr := validateGitEnvelope(root)
	if executionErr != nil {
		return 2, executionErr.Error()
	}
	if !report.Valid || len(findings) != 0 {
		return 1, "managed serve requires a valid committed Vault"
	}
	clean, err := runGit(root, nil, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return 2, "cannot inspect Vault worktree"
	}
	if strings.TrimSpace(clean) != "" {
		return 1, "managed serve requires a clean Vault worktree"
	}
	return 0, ""
}

func pathWithinCanonical(root, candidate string) (bool, error) {
	return pathguard.Contains(root, candidate)
}

func validateHTTPExposure(listen string, oauthEnabled bool) error {
	if oauthEnabled {
		return nil
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(listen))
	if err != nil {
		return fmt.Errorf("invalid HTTP listen address")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("unauthenticated HTTP serve is restricted to loopback; enable OAuth for non-loopback listen")
}

func newHTTPServer(listen string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: listen, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

func splitCSV(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if clean := strings.TrimSpace(item); clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

func initializedVault(root string) bool {
	report := vaultvalidate.Validate(root)
	if !report.Valid {
		return false
	}
	findings, err := validateGitEnvelope(root)
	return err == nil && len(findings) == 0
}

func validateGitEnvelope(root string) ([]vaultvalidate.Finding, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, errors.New("cannot resolve Vault path")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return []vaultvalidate.Finding{{Code: "git_repository", Message: "Vault must be a Git repository root with a commit"}}, nil
	}
	top, err := runGit(resolved, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		if isExitError(err) {
			return []vaultvalidate.Finding{{Code: "git_repository", Message: "Vault must be a Git repository root with a commit"}}, nil
		}
		return nil, errors.New("cannot inspect Git repository")
	}
	topResolved, evalErr := filepath.EvalSymlinks(strings.TrimSpace(top))
	if evalErr != nil {
		return nil, errors.New("cannot resolve Git repository root")
	}
	if filepath.Clean(topResolved) != filepath.Clean(resolved) {
		return []vaultvalidate.Finding{{Code: "git_root", Message: "validation path must be the Vault Git repository root"}}, nil
	}
	if _, err := runGit(resolved, nil, "rev-parse", "--verify", "HEAD^{commit}"); err != nil {
		if isExitError(err) {
			return []vaultvalidate.Finding{{Code: "git_head", Message: "Vault Git repository must have an initial commit"}}, nil
		}
		return nil, errors.New("cannot inspect Git HEAD")
	}
	return nil, nil
}

func insideExistingGitWorktree(target string) (bool, error) {
	probe := target
	for {
		if info, err := os.Stat(probe); err == nil && info.IsDir() {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return false, nil
		}
		probe = parent
	}
	top, err := runGit(probe, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		if isExitError(err) {
			return false, nil
		}
		return false, errors.New("cannot inspect parent Git repository")
	}
	topResolved, err := filepath.EvalSymlinks(strings.TrimSpace(top))
	if err != nil {
		return false, errors.New("cannot resolve parent Git repository")
	}
	inside, err := pathguard.Contains(topResolved, target)
	if err != nil {
		return false, errors.New("cannot resolve init target")
	}
	return inside, nil
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func runGit(root string, env []string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", commandArgs...)
	if env == nil {
		env = gitEnvironment()
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func gitEnvironment() []string {
	base := make([]string, 0, len(os.Environ())+3)
	for _, item := range os.Environ() {
		key := item
		if at := strings.IndexByte(item, '='); at >= 0 {
			key = item[:at]
		}
		if strings.HasPrefix(key, "GIT_") {
			continue
		}
		base = append(base, item)
	}
	return append(base,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	)
}

func isExitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func sortFindings(findings []vaultvalidate.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		return a.Message < b.Message
	})
}
