package srv

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"context"
	"net/url"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
)

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	StaticDir    string
	BaseURL      string  // public base URL for links
	Storage      Storage // local FS or S3 (see storage.go)
}

// auth

type pageData struct {
	Hostname   string
	Now        string
	UserEmail  string
	VisitCount int64
	LoginURL   string
	LogoutURL  string
	Headers    []headerEntry
}

type headerEntry struct {
	Name       string
	Values     []string
	AddedByExe bool
}

const sessionCookie = "rh_session"
const sessionTTL = 30 * 24 * time.Hour

func New(dbPath, hostname string) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	srv := &Server{
		Hostname:     hostname,
		TemplatesDir: filepath.Join(baseDir, "templates"),
		StaticDir:    filepath.Join(baseDir, "static"),
	}
	if err := srv.setUpDatabase(dbPath); err != nil {
		return nil, err
	}
	return srv, nil
}

// hashToken returns the hex sha256 of a raw secret. Tokens and session ids are
// stored hashed so a leaked DB does not leak credentials.
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h)
}

func randomToken(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%x", prefix, b), nil
}

// validPassword checks the single admin password stored in config.
func (s *Server) validPassword(password string) bool {
	q := dbgen.New(s.DB)
	ctx := context.Background()
	hash, err := q.GetConfig(ctx, "admin_password_hash")
	if err != nil || hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// requireUI is middleware for browser pages: session cookie → login redirect.
func (s *Server) requireUI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validSession(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// requireAPI is middleware for programmatic endpoints: Bearer token → 401.
func (s *Server) requireAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="release-hub"`)
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, prefix)
		q := dbgen.New(s.DB)
		_, err := q.ApiTokenByHash(r.Context(), hashToken(token))
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		now := time.Now()
		_ = q.TouchApiToken(r.Context(), dbgen.TouchApiTokenParams{LastUsedAt: &now, TokenHash: hashToken(token)})
		next(w, r)
	}
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return false
	}
	q := dbgen.New(s.DB)
	sess, err := q.SessionByHash(r.Context(), hashToken(c.Value))
	if err != nil {
		return false
	}
	return time.Now().Before(sess.ExpiresAt)
}

func (s *Server) setSession(w http.ResponseWriter, r *http.Request) error {
	token, err := randomToken("sess")
	if err != nil {
		return err
	}
	q := dbgen.New(s.DB)
	now := time.Now()
	if err := q.CreateSession(r.Context(), dbgen.CreateSessionParams{TokenHash: hashToken(token), CreatedAt: now, ExpiresAt: now.Add(sessionTTL)}); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.HasPrefix(s.BaseURL, "https://"),
		MaxAge:   int(sessionTTL.Seconds()),
	})
	return nil
}

func (s *Server) clearSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = dbgen.New(s.DB).DeleteSession(r.Context(), hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
}



func loginURLForRequest(r *http.Request) string {
	// Send the user back to this page after login, but use only the path and
	// drop the query. The query can carry exe.dev's reserved "redirect" param;
	// folding that back into a new redirect param would let a crawler following
	// the login link nest (and re-encode) the whole URL each hop, growing it
	// without bound. A bare path can't loop.
	v := url.Values{}
	v.Set("redirect", r.URL.Path)
	return "/__exe.dev/login?" + v.Encode()
}



func mainDomainFromHost(h string) string {
	host, port, err := net.SplitHostPort(h)
	if err != nil {
		host = strings.TrimSpace(h)
	}
	if port != "" {
		port = ":" + port
	}
	// Check for exe.cloud-based domains (dev mode)
	if strings.HasSuffix(host, ".exe.cloud") || host == "exe.cloud" {
		return "exe.cloud" + port
	}
	// Check for exe.dev-based domains (production)
	if strings.HasSuffix(host, ".exe.dev") || host == "exe.dev" {
		return "exe.dev"
	}
	// Return as-is for custom domains
	return host
}

// SetupDatabase initializes the database connection and runs migrations
func (s *Server) setUpDatabase(dbPath string) error {
	wdb, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	s.DB = wdb
	if err := db.RunMigrations(wdb); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

// renderTemplate parses base + page template. Pages define a "content" block
// and expect a {{template "base.html" .}} header line.
func renderTemplate(w http.ResponseWriter, dir, name string, data any) error {
	base := filepath.Join(dir, "base.html")
	page := filepath.Join(dir, name)
	tmpl, err := template.ParseFiles(page, base)
	if err != nil {
		return fmt.Errorf("parse %q: %w", name, err)
	}
	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		return fmt.Errorf("execute %q: %w", name, err)
	}
	return nil
}

// flash reads and clears the short-lived rh_flash cookie.
func flash(r *http.Request) string {
	c, err := r.Cookie("rh_flash")
	if err != nil {
		return ""
	}
	return c.Value
}

// Serve starts the HTTP server with the configured routes.
func (s *Server) Serve(addr string) error {
	mux := http.NewServeMux()

	// UI (session auth; setup/login always reachable)
	mux.HandleFunc("GET /setup", s.handleFirstRun)
	mux.HandleFunc("POST /setup", s.handleFirstRun)
	mux.HandleFunc("GET /login", s.handleLogin)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("GET /logout", s.handleLogout)
	mux.HandleFunc("GET /{$}", s.requireUI(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(withFlash(r, flash(r)))
		s.handleHome(w, r)
	}))
	mux.HandleFunc("POST /apps", s.requireUI(s.handleCreateAppUI))
	mux.HandleFunc("POST /tokens", s.requireUI(s.handleCreateTokenUI))
	mux.HandleFunc("GET /apps/{slug}", s.requireUI(s.handleAppDetail))
	mux.HandleFunc("GET /settings", s.requireUI(s.handleSettings))
	mux.HandleFunc("POST /settings", s.requireUI(s.handleSettings))

	// API (bearer auth)
	mux.HandleFunc("GET /api/apps", s.requireAPI(s.handleApiListApps))
	mux.HandleFunc("POST /api/apps", s.requireAPI(s.handleApiCreateApp))
	mux.HandleFunc("POST /api/apps/{slug}/releases", s.requireAPI(s.handleApiUpload))
	mux.HandleFunc("GET /api/apps/{slug}/releases", s.requireAPI(s.handleApiReleases))
	mux.HandleFunc("POST /api/tokens", s.requireAPI(s.handleApiCreateToken))
	mux.HandleFunc("POST /api/apps/{slug}/play", s.requireAPI(s.handleApiSetPlay))
	mux.HandleFunc("POST /api/apps/{slug}/signing", s.requireAPI(s.handleApiSetSigning))
	mux.HandleFunc("GET /api/apps/{slug}/signing", s.requireAPI(s.handleApiGetSigning))
	mux.HandleFunc("POST /api/apps/{slug}/signing/delete", s.requireAPI(s.handleApiDeleteSigning))

	// Public (devices): manifest + artifact download need no auth.
	mux.HandleFunc("GET /api/apps/{slug}/manifest", s.handleManifest)
	mux.HandleFunc("GET /artifacts/{slug}/{file}", s.handleArtifact)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	stype := "local"
	if _, ok := s.Storage.(*S3Storage); ok {
		stype = "s3"
	}
	slog.Info("starting release-hub", "addr", addr, "storage", stype)
	return http.ListenAndServe(addr, mux)
}

func withFlash(r *http.Request, msg string) context.Context {
	return context.WithValue(r.Context(), flashKey{}, msg)
}

type flashKey struct{}

func flashFrom(r *http.Request) string {
	v, _ := r.Context().Value(flashKey{}).(string)
	return v
}

func buildHeaderEntries(r *http.Request) []headerEntry {
	if r == nil {
		return nil
	}

	headers := make([]headerEntry, 0, len(r.Header)+1)
	for name, values := range r.Header {
		lower := strings.ToLower(name)
		headers = append(headers, headerEntry{
			Name:       name,
			Values:     values,
			AddedByExe: strings.HasPrefix(lower, "x-exedev-") || strings.HasPrefix(lower, "x-forwarded-"),
		})
	}
	if r.Host != "" {
		headers = append(headers, headerEntry{
			Name:   "Host",
			Values: []string{r.Host},
		})
	}

	sort.Slice(headers, func(i, j int) bool {
		return strings.ToLower(headers[i].Name) < strings.ToLower(headers[j].Name)
	})
	return headers
}
