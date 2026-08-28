// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package oauthserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	_ "modernc.org/sqlite"
)

const (
	ScopeMemory        = "memory"
	ScopeOfflineAccess = "offline_access"
)

var (
	defaultPendingTTL      = 10 * time.Minute
	defaultCodeTTL         = 5 * time.Minute
	defaultAccessTTL       = 1 * time.Hour
	defaultRefreshTTL      = 30 * 24 * time.Hour
	loginFailureWindow     = 300 * time.Second
	loginFailureLimit      = 5
	loginRetryAfterSeconds = 60
)

type Config struct {
	Issuer           string
	ClientID         string
	RedirectURIs     []string
	Username         string
	PasswordFile     string
	ClientSecretFile string
	StaticTokenFile  string
	StateDB          string

	PendingTTL time.Duration
	CodeTTL    time.Duration
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	Now        func() time.Time
	Random     io.Reader
}

type Server struct {
	cfg          Config
	resource     string
	password     string
	clientSecret string
	staticToken  string
	db           *sql.DB
	now          func() time.Time
	random       io.Reader
	loginMu      sync.Mutex
}

func Open(cfg Config) (*Server, error) {
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	password, err := readCredentialFile(cfg.PasswordFile, "OAuth password")
	if err != nil {
		return nil, err
	}
	clientSecret, err := readOptionalCredentialFile(cfg.ClientSecretFile, "OAuth client secret")
	if err != nil {
		return nil, err
	}
	staticToken, err := readStaticTokenFile(cfg.StaticTokenFile)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.StateDB), 0o700); err != nil {
		return nil, fmt.Errorf("create OAuth state directory: %w", err)
	}
	db, err := sql.Open("sqlite", cfg.StateDB)
	if err != nil {
		return nil, fmt.Errorf("open OAuth state DB: %w", err)
	}
	db.SetMaxOpenConns(1)
	server := &Server{
		cfg:          cfg,
		resource:     strings.TrimRight(cfg.Issuer, "/") + "/mcp",
		password:     password,
		clientSecret: clientSecret,
		staticToken:  staticToken,
		db:           db,
		now:          cfg.Now,
		random:       cfg.Random,
	}
	if err := server.initDB(); err != nil {
		db.Close()
		return nil, err
	}
	_ = os.Chmod(cfg.StateDB, 0o600)
	return server, nil
}

func readCredentialFile(path, label string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s file: %w", label, err)
	}
	if len(raw) == 0 || len(raw) > 4096 {
		return "", fmt.Errorf("%s file must contain 1 to 4096 bytes", label)
	}
	value := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("%s must be one non-empty line", label)
	}
	return value, nil
}

func readOptionalCredentialFile(path, label string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s path must be absolute", label)
	}
	return readCredentialFile(path, label)
}

func readStaticTokenFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("deployment auth token path must be absolute")
	}
	value, err := readCredentialFile(path, "deployment auth token")
	if err != nil {
		return "", err
	}
	if len(value) != 64 {
		return "", fmt.Errorf("deployment auth token must be exactly 64 hexadecimal characters")
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return "", fmt.Errorf("deployment auth token must be exactly 64 hexadecimal characters")
		}
	}
	return value, nil
}

func validateConfig(cfg *Config) error {
	cfg.Issuer = strings.TrimRight(strings.TrimSpace(cfg.Issuer), "/")
	if err := validateIssuer(cfg.Issuer); err != nil {
		return err
	}
	if cfg.ClientID == "" || strings.ContainsAny(cfg.ClientID, "\r\n") {
		return fmt.Errorf("OAuth client_id is required")
	}
	if cfg.Username == "" || strings.ContainsAny(cfg.Username, "\r\n") {
		return fmt.Errorf("OAuth username is required")
	}
	if cfg.PasswordFile == "" {
		return fmt.Errorf("OAuth password file is required")
	}
	if cfg.StateDB == "" {
		return fmt.Errorf("OAuth state DB is required")
	}
	if len(cfg.RedirectURIs) == 0 {
		return fmt.Errorf("at least one OAuth redirect URI is required")
	}
	seen := map[string]bool{}
	for _, raw := range cfg.RedirectURIs {
		if err := validateRedirectURI(raw); err != nil {
			return err
		}
		if seen[raw] {
			return fmt.Errorf("duplicate OAuth redirect URI")
		}
		seen[raw] = true
	}
	if cfg.PendingTTL <= 0 {
		cfg.PendingTTL = defaultPendingTTL
	}
	if cfg.CodeTTL <= 0 {
		cfg.CodeTTL = defaultCodeTTL
	}
	if cfg.AccessTTL <= 0 {
		cfg.AccessTTL = defaultAccessTTL
	}
	if cfg.RefreshTTL <= 0 {
		cfg.RefreshTTL = defaultRefreshTTL
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.Random == nil {
		cfg.Random = rand.Reader
	}
	return nil
}

func validateIssuer(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("OAuth issuer must be an origin URL")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("OAuth issuer must use HTTPS except on loopback")
}

func validateRedirectURI(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("invalid OAuth redirect URI")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("OAuth redirect URI must use HTTPS or loopback HTTP")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) Close() error     { return s.db.Close() }
func (s *Server) Issuer() string   { return s.cfg.Issuer }
func (s *Server) Resource() string { return s.resource }
func (s *Server) ResourceMetadataURL() string {
	return s.cfg.Issuer + "/.well-known/oauth-protected-resource/mcp"
}

func (s *Server) initDB() error {
	const schema = `
CREATE TABLE IF NOT EXISTS pending_authorizations (
  transaction_id TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  state TEXT NOT NULL,
  scopes TEXT NOT NULL,
  code_challenge TEXT NOT NULL,
  redirect_uri TEXT NOT NULL,
  resource TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  csrf_hash TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS authorization_codes (
  code_hash TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  scopes TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  code_challenge TEXT NOT NULL,
  redirect_uri TEXT NOT NULL,
  resource TEXT NOT NULL,
  subject TEXT NOT NULL,
  used INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  used_at INTEGER
);
CREATE TABLE IF NOT EXISTS access_tokens (
  token_hash TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  scopes TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  resource TEXT NOT NULL,
  subject TEXT NOT NULL,
  family_id TEXT NOT NULL,
  revoked INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS refresh_tokens (
  token_hash TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  scopes TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  resource TEXT NOT NULL,
  subject TEXT NOT NULL,
  family_id TEXT NOT NULL,
  revoked INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  rotated_at INTEGER
);
CREATE TABLE IF NOT EXISTS login_attempts (
  key TEXT PRIMARY KEY,
  window_started INTEGER NOT NULL,
  failures INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_access_tokens_family ON access_tokens(family_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family ON refresh_tokens(family_id);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("initialize OAuth state DB: %w", err)
	}
	var quick string
	if err := s.db.QueryRow("PRAGMA quick_check").Scan(&quick); err != nil || quick != "ok" {
		if err == nil {
			err = fmt.Errorf("quick_check=%s", quick)
		}
		return fmt.Errorf("OAuth state DB integrity check failed: %w", err)
	}
	return nil
}

func (s *Server) ProtectedResourceMetadataHandler() http.Handler {
	metadata := &oauthex.ProtectedResourceMetadata{
		Resource:               s.resource,
		AuthorizationServers:   []string{s.cfg.Issuer},
		ScopesSupported:        []string{ScopeMemory},
		BearerMethodsSupported: []string{"header"},
	}
	return mcpauth.ProtectedResourceMetadataHandler(metadata)
}

func (s *Server) AuthorizationServerMetadataHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		methods := []string{"none"}
		if s.clientSecret != "" {
			methods = []string{"client_secret_post", "client_secret_basic"}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                                s.cfg.Issuer,
			"authorization_endpoint":                s.cfg.Issuer + "/authorize",
			"token_endpoint":                        s.cfg.Issuer + "/token",
			"scopes_supported":                      []string{ScopeMemory, ScopeOfflineAccess},
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"token_endpoint_auth_methods_supported": methods,
			"revocation_endpoint":                   s.cfg.Issuer + "/revoke",
			"code_challenge_methods_supported":      []string{"S256"},
		})
	})
}

func (s *Server) Protect(handler http.Handler) http.Handler {
	return mcpauth.RequireBearerToken(s.VerifyToken, &mcpauth.RequireBearerTokenOptions{
		Scopes:              []string{ScopeMemory},
		ResourceMetadataURL: s.ResourceMetadataURL(),
	})(handler)
}

func (s *Server) VerifyToken(ctx context.Context, token string, _ *http.Request) (*mcpauth.TokenInfo, error) {
	if s.staticToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.staticToken)) == 1 {
		return &mcpauth.TokenInfo{Scopes: []string{ScopeMemory}, Expiration: time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC), UserID: "local-deployment", Extra: map[string]any{"resource": s.resource}}, nil
	}
	var scopes, resource, subject string
	var expiresAt int64
	var revoked int
	err := s.db.QueryRowContext(ctx, `SELECT scopes, expires_at, resource, subject, revoked FROM access_tokens WHERE token_hash=?`, tokenHash(token)).Scan(&scopes, &expiresAt, &resource, &subject, &revoked)
	if errors.Is(err, sql.ErrNoRows) || revoked != 0 || resource != s.resource || expiresAt <= s.now().Unix() {
		return nil, mcpauth.ErrInvalidToken
	}
	if err != nil {
		return nil, fmt.Errorf("verify OAuth token: %w", err)
	}
	return &mcpauth.TokenInfo{
		Scopes:     strings.Fields(scopes),
		Expiration: time.Unix(expiresAt, 0),
		UserID:     subject,
		Extra:      map[string]any{"resource": resource},
	}, nil
}

func (s *Server) AuthorizeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			oauthError(w, http.StatusMethodNotAllowed, "invalid_request", "authorization endpoint requires GET", r.URL.Query().Get("state"))
			return
		}
		q := r.URL.Query()
		state := q.Get("state")
		if q.Get("client_id") != s.cfg.ClientID {
			oauthError(w, http.StatusBadRequest, "invalid_request", "unknown client_id", state)
			return
		}
		if q.Get("response_type") != "code" {
			oauthError(w, http.StatusBadRequest, "unsupported_response_type", "response_type must be code", state)
			return
		}
		redirect := q.Get("redirect_uri")
		if !contains(s.cfg.RedirectURIs, redirect) {
			oauthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is not registered for client", state)
			return
		}
		challenge := q.Get("code_challenge")
		if challenge == "" || q.Get("code_challenge_method") != "S256" {
			oauthError(w, http.StatusBadRequest, "invalid_request", "PKCE S256 code_challenge is required", state)
			return
		}
		if q.Get("resource") != s.resource {
			oauthError(w, http.StatusBadRequest, "invalid_target", "resource does not match this MCP server", state)
			return
		}
		scopes, err := normalizeScopes(q.Get("scope"), true)
		if err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_scope", err.Error(), state)
			return
		}
		transaction, err := s.randomToken()
		if err != nil {
			http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
			return
		}
		expires := s.now().Add(s.cfg.PendingTTL).Unix()
		if _, err := s.db.ExecContext(r.Context(), `INSERT INTO pending_authorizations(transaction_id,client_id,state,scopes,code_challenge,redirect_uri,resource,expires_at) VALUES(?,?,?,?,?,?,?,?)`, transaction, s.cfg.ClientID, state, strings.Join(scopes, " "), challenge, redirect, s.resource, expires); err != nil {
			http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
			return
		}
		login := s.cfg.Issuer + "/oauth/login?transaction=" + url.QueryEscape(transaction)
		http.Redirect(w, r, login, http.StatusFound)
	})
}

func (s *Server) LoginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.loginGET(w, r)
		case http.MethodPost:
			s.loginPOST(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

type pendingAuthorization struct {
	Transaction string
	ClientID    string
	State       string
	Scopes      string
	Challenge   string
	RedirectURI string
	Resource    string
	ExpiresAt   int64
	CSRFHash    string
}

func (s *Server) pending(ctx context.Context, transaction string) (pendingAuthorization, error) {
	var p pendingAuthorization
	err := s.db.QueryRowContext(ctx, `SELECT transaction_id,client_id,state,scopes,code_challenge,redirect_uri,resource,expires_at,csrf_hash FROM pending_authorizations WHERE transaction_id=?`, transaction).Scan(&p.Transaction, &p.ClientID, &p.State, &p.Scopes, &p.Challenge, &p.RedirectURI, &p.Resource, &p.ExpiresAt, &p.CSRFHash)
	if err != nil {
		return p, err
	}
	if p.ExpiresAt <= s.now().Unix() {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM pending_authorizations WHERE transaction_id=?`, transaction)
		return p, sql.ErrNoRows
	}
	return p, nil
}

func (s *Server) loginGET(w http.ResponseWriter, r *http.Request) {
	transaction := r.URL.Query().Get("transaction")
	if transaction == "" {
		http.Error(w, "Invalid authorization request", http.StatusBadRequest)
		return
	}
	if _, err := s.pending(r.Context(), transaction); err != nil {
		http.Error(w, "Invalid authorization request", http.StatusBadRequest)
		return
	}
	csrf, err := s.randomToken()
	if err != nil {
		http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
		return
	}
	if _, err := s.db.ExecContext(r.Context(), `UPDATE pending_authorizations SET csrf_hash=? WHERE transaction_id=?`, tokenHash(csrf), transaction); err != nil {
		http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "oauth_csrf", Value: csrf, Path: "/oauth/login", MaxAge: 600, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>V-Memory authorization</title></head>
<body>
<h1>V-Memory authorization</h1>
<p>Sign in to authorize the configured MCP client.</p>
<form method="post" action="/oauth/login">
<input type="hidden" name="transaction" value="%s">
<input type="hidden" name="csrf" value="%s">
<label>Username <input name="username" autocomplete="username" required></label>
<label>Password <input type="password" name="password" autocomplete="current-password" required></label>
<button type="submit">Authorize</button>
</form>
</body></html>`, html.EscapeString(transaction), html.EscapeString(csrf))
}

func (s *Server) loginPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid authorization request", http.StatusBadRequest)
		return
	}
	p, err := s.pending(r.Context(), r.Form.Get("transaction"))
	if err != nil {
		http.Error(w, "Invalid authorization request", http.StatusBadRequest)
		return
	}
	cookie, err := r.Cookie("oauth_csrf")
	csrf := r.Form.Get("csrf")
	if err != nil || csrf == "" || cookie.Value != csrf || subtle.ConstantTimeCompare([]byte(tokenHash(csrf)), []byte(p.CSRFHash)) != 1 {
		http.Error(w, "Invalid CSRF token", http.StatusBadRequest)
		return
	}
	clientHost := requestClientHost(r)
	s.loginMu.Lock()
	allowed, rateErr := s.loginAttemptAllowed(r.Context(), clientHost)
	if rateErr != nil {
		s.loginMu.Unlock()
		http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
		return
	}
	if !allowed {
		s.loginMu.Unlock()
		w.Header().Set("Retry-After", fmt.Sprintf("%d", loginRetryAfterSeconds))
		http.Error(w, "Too many failed login attempts", http.StatusTooManyRequests)
		return
	}
	credentialsMatch := subtle.ConstantTimeCompare([]byte(r.Form.Get("username")), []byte(s.cfg.Username)) == 1 && subtle.ConstantTimeCompare([]byte(r.Form.Get("password")), []byte(s.password)) == 1
	if !credentialsMatch {
		recordErr := s.recordLoginFailure(r.Context(), clientHost)
		s.loginMu.Unlock()
		if recordErr != nil {
			http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
			return
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}
	clearErr := s.clearLoginFailures(r.Context(), clientHost)
	s.loginMu.Unlock()
	if clearErr != nil {
		http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
		return
	}
	code, err := s.randomToken()
	if err != nil {
		http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
		return
	}
	now := s.now().Unix()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO authorization_codes(code_hash,client_id,scopes,expires_at,code_challenge,redirect_uri,resource,subject,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, tokenHash(code), p.ClientID, p.Scopes, s.now().Add(s.cfg.CodeTTL).Unix(), p.Challenge, p.RedirectURI, p.Resource, s.cfg.Username, now); err != nil {
		http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM pending_authorizations WHERE transaction_id=?`, p.Transaction); err != nil {
		http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "authorization state unavailable", http.StatusInternalServerError)
		return
	}
	redirect, _ := url.Parse(p.RedirectURI)
	q := redirect.Query()
	q.Set("code", code)
	if p.State != "" {
		q.Set("state", p.State)
	}
	q.Set("iss", s.cfg.Issuer)
	redirect.RawQuery = q.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (s *Server) clientAuthorized(r *http.Request) bool {
	form := r.Form
	if s.clientSecret == "" {
		return form.Get("client_id") == s.cfg.ClientID
	}
	if r.Header.Get("Authorization") != "" {
		if form.Get("client_id") != "" || form.Get("client_secret") != "" {
			return false
		}
		clientID, clientSecret, ok := r.BasicAuth()
		if !ok {
			return false
		}
		var err error
		clientID, err = url.QueryUnescape(clientID)
		if err != nil {
			return false
		}
		clientSecret, err = url.QueryUnescape(clientSecret)
		if err != nil {
			return false
		}
		return clientID == s.cfg.ClientID && subtle.ConstantTimeCompare([]byte(clientSecret), []byte(s.clientSecret)) == 1
	}
	return form.Get("client_id") == s.cfg.ClientID && subtle.ConstantTimeCompare([]byte(form.Get("client_secret")), []byte(s.clientSecret)) == 1
}

func (s *Server) TokenHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			oauthError(w, http.StatusBadRequest, "invalid_request", "invalid token request", "")
			return
		}
		if !s.clientAuthorized(r) {
			oauthError(w, http.StatusUnauthorized, "invalid_client", "invalid client credentials", "")
			return
		}
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			s.exchangeCode(w, r)
		case "refresh_token":
			s.exchangeRefresh(w, r)
		default:
			oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "unsupported grant_type", "")
		}
	})
}

type codeRecord struct {
	ClientID, Scopes, Challenge, RedirectURI, Resource, Subject string
	ExpiresAt                                                   int64
	Used                                                        int
}

func (s *Server) exchangeCode(w http.ResponseWriter, r *http.Request) {
	if r.Form.Get("resource") != s.resource {
		oauthError(w, http.StatusBadRequest, "invalid_target", "resource does not match this MCP server", "")
		return
	}
	code := r.Form.Get("code")
	verifier := r.Form.Get("code_verifier")
	if code == "" || verifier == "" {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "authorization code and code_verifier are required", "")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "token state unavailable", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var rec codeRecord
	err = tx.QueryRowContext(r.Context(), `SELECT client_id,scopes,expires_at,code_challenge,redirect_uri,resource,subject,used FROM authorization_codes WHERE code_hash=?`, tokenHash(code)).Scan(&rec.ClientID, &rec.Scopes, &rec.ExpiresAt, &rec.Challenge, &rec.RedirectURI, &rec.Resource, &rec.Subject, &rec.Used)
	if err != nil || rec.Used != 0 || rec.ExpiresAt <= s.now().Unix() || rec.ClientID != s.cfg.ClientID || rec.RedirectURI != r.Form.Get("redirect_uri") || rec.Resource != s.resource || !verifyPKCE(verifier, rec.Challenge) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is invalid", "")
		return
	}
	now := s.now().Unix()
	if _, err := tx.ExecContext(r.Context(), `UPDATE authorization_codes SET used=1, used_at=? WHERE code_hash=? AND used=0`, now, tokenHash(code)); err != nil {
		http.Error(w, "token state unavailable", http.StatusInternalServerError)
		return
	}
	response, err := s.issueTokens(r.Context(), tx, rec.ClientID, rec.Scopes, rec.Resource, rec.Subject, "")
	if err != nil {
		http.Error(w, "token state unavailable", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "token state unavailable", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type refreshRecord struct {
	ClientID, Scopes, Resource, Subject, FamilyID string
	ExpiresAt                                     int64
	Revoked                                       int
}

func (s *Server) exchangeRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Form.Get("resource") != s.resource {
		oauthError(w, http.StatusBadRequest, "invalid_target", "resource does not match this MCP server", "")
		return
	}
	raw := r.Form.Get("refresh_token")
	if raw == "" {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "refresh_token is required", "")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "token state unavailable", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var rec refreshRecord
	err = tx.QueryRowContext(r.Context(), `SELECT client_id,scopes,expires_at,resource,subject,family_id,revoked FROM refresh_tokens WHERE token_hash=?`, tokenHash(raw)).Scan(&rec.ClientID, &rec.Scopes, &rec.ExpiresAt, &rec.Resource, &rec.Subject, &rec.FamilyID, &rec.Revoked)
	if err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "refresh token is invalid", "")
		return
	}
	if rec.Revoked != 0 {
		if rec.ClientID == s.cfg.ClientID && rec.Resource == s.resource && rec.FamilyID != "" {
			if err := revokeTokenFamily(r.Context(), tx, rec.ClientID, rec.FamilyID); err != nil {
				http.Error(w, "token state unavailable", http.StatusInternalServerError)
				return
			}
			if err := tx.Commit(); err != nil {
				http.Error(w, "token state unavailable", http.StatusInternalServerError)
				return
			}
		}
		oauthError(w, http.StatusBadRequest, "invalid_grant", "refresh token is invalid", "")
		return
	}
	if rec.ExpiresAt <= s.now().Unix() || rec.ClientID != s.cfg.ClientID || rec.Resource != s.resource {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "refresh token is invalid", "")
		return
	}
	scopes := rec.Scopes
	if requested := r.Form.Get("scope"); requested != "" {
		normalized, err := normalizeScopes(requested, true)
		if err != nil || !scopeSubset(normalized, strings.Fields(rec.Scopes)) {
			oauthError(w, http.StatusBadRequest, "invalid_scope", "requested scope exceeds original grant", "")
			return
		}
		scopes = strings.Join(normalized, " ")
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE refresh_tokens SET revoked=1, rotated_at=? WHERE token_hash=? AND revoked=0`, s.now().Unix(), tokenHash(raw)); err != nil {
		http.Error(w, "token state unavailable", http.StatusInternalServerError)
		return
	}
	response, err := s.issueTokens(r.Context(), tx, rec.ClientID, scopes, rec.Resource, rec.Subject, rec.FamilyID)
	if err != nil {
		http.Error(w, "token state unavailable", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "token state unavailable", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func revokeTokenFamily(ctx context.Context, tx *sql.Tx, clientID, familyID string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE access_tokens SET revoked=1 WHERE client_id=? AND family_id=?`, clientID, familyID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked=1 WHERE client_id=? AND family_id=?`, clientID, familyID); err != nil {
		return err
	}
	return nil
}

func requestClientHost(r *http.Request) string {
	host := strings.TrimSpace(r.RemoteAddr)
	if parsed, _, err := net.SplitHostPort(host); err == nil && parsed != "" {
		return parsed
	}
	if host == "" {
		return "unknown"
	}
	return host
}

func (s *Server) loginAttemptAllowed(ctx context.Context, clientHost string) (bool, error) {
	var started int64
	var failures int
	err := s.db.QueryRowContext(ctx, `SELECT window_started, failures FROM login_attempts WHERE key=?`, tokenHash(clientHost)).Scan(&started, &failures)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if s.now().Unix()-started >= int64(loginFailureWindow/time.Second) {
		return true, nil
	}
	return failures < loginFailureLimit, nil
}

func (s *Server) recordLoginFailure(ctx context.Context, clientHost string) error {
	key := tokenHash(clientHost)
	now := s.now().Unix()
	var started int64
	var failures int
	err := s.db.QueryRowContext(ctx, `SELECT window_started, failures FROM login_attempts WHERE key=?`, key).Scan(&started, &failures)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && now-started >= int64(loginFailureWindow/time.Second)) {
		_, err = s.db.ExecContext(ctx, `INSERT INTO login_attempts(key,window_started,failures) VALUES(?,?,1) ON CONFLICT(key) DO UPDATE SET window_started=excluded.window_started, failures=1`, key, now)
		return err
	}
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE login_attempts SET failures=failures+1 WHERE key=?`, key)
	return err
}

func (s *Server) clearLoginFailures(ctx context.Context, clientHost string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_attempts WHERE key=?`, tokenHash(clientHost))
	return err
}

func (s *Server) issueTokens(ctx context.Context, tx *sql.Tx, clientID, scopes, resource, subject, familyID string) (map[string]any, error) {
	access, err := s.randomToken()
	if err != nil {
		return nil, err
	}
	refresh, err := s.randomToken()
	if err != nil {
		return nil, err
	}
	if familyID == "" {
		familyID, err = s.randomToken()
		if err != nil {
			return nil, err
		}
	}
	now := s.now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO access_tokens(token_hash,client_id,scopes,expires_at,resource,subject,family_id,created_at) VALUES(?,?,?,?,?,?,?,?)`, tokenHash(access), clientID, scopes, now.Add(s.cfg.AccessTTL).Unix(), resource, subject, familyID, now.Unix()); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO refresh_tokens(token_hash,client_id,scopes,expires_at,resource,subject,family_id,created_at) VALUES(?,?,?,?,?,?,?,?)`, tokenHash(refresh), clientID, scopes, now.Add(s.cfg.RefreshTTL).Unix(), resource, subject, familyID, now.Unix()); err != nil {
		return nil, err
	}
	return map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int64(s.cfg.AccessTTL / time.Second),
		"scope":         scopes,
		"refresh_token": refresh,
	}, nil
}

func (s *Server) RevokeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil || !s.clientAuthorized(r) {
			oauthError(w, http.StatusUnauthorized, "invalid_client", "invalid client credentials", "")
			return
		}
		h := tokenHash(r.Form.Get("token"))
		_, _ = s.db.ExecContext(r.Context(), `UPDATE access_tokens SET revoked=1 WHERE token_hash=? AND client_id=?`, h, s.cfg.ClientID)
		_, _ = s.db.ExecContext(r.Context(), `UPDATE refresh_tokens SET revoked=1 WHERE token_hash=? AND client_id=?`, h, s.cfg.ClientID)
		w.WriteHeader(http.StatusOK)
	})
}

func (s *Server) randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := io.ReadFull(s.random, buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func tokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func verifyPKCE(verifier, expected string) bool {
	sum := sha256.Sum256([]byte(verifier))
	actual := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func normalizeScopes(raw string, requireMemory bool) ([]string, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil, fmt.Errorf("scope is required")
	}
	allowed := map[string]bool{ScopeMemory: true, ScopeOfflineAccess: true}
	seen := map[string]bool{}
	for _, scope := range fields {
		if !allowed[scope] {
			return nil, fmt.Errorf("unsupported scope")
		}
		seen[scope] = true
	}
	if requireMemory && !seen[ScopeMemory] {
		return nil, fmt.Errorf("memory scope is required")
	}
	out := make([]string, 0, len(seen))
	for _, scope := range []string{ScopeMemory, ScopeOfflineAccess} {
		if seen[scope] {
			out = append(out, scope)
		}
	}
	return out, nil
}

func scopeSubset(requested, original []string) bool {
	set := map[string]bool{}
	for _, s := range original {
		set[s] = true
	}
	for _, s := range requested {
		if !set[s] {
			return false
		}
	}
	return true
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func oauthError(w http.ResponseWriter, status int, code, description, state string) {
	value := map[string]any{"error": code, "error_description": description}
	if state != "" {
		value["state"] = state
	}
	writeJSON(w, status, value)
}

func SortedScopes(raw string) []string {
	out := strings.Fields(raw)
	sort.Strings(out)
	return out
}
