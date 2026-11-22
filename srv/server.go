package srv

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"srv.exe.dev/db"
)

type Server struct {
	DB       *sql.DB
	Hostname string
	baseURL  string       // public base URL for links
	storage  Storage      // local FS or S3 (see storage.go)
	uiFS     fs.FS        // built React UI; nil = the copy embedded from web/dist
	oidcHTTP *http.Client // outbound client for OIDC discovery/JWKS
	oidcJWKS *jwksCache
	vault    *opVault // 1Password Connect (vault.go); nil = keys sealed in the DB only
}

// Options configures a Server at construction time (listmonk-style config
// injection instead of exported mutable fields).
type Options struct {
	DBPath   string  // sqlite database path
	Hostname string  // for display/debugging
	BaseURL  string  // public base URL for manifest/download links
	Storage  Storage // local FS or S3 (see storage.go); required
	UI       fs.FS   // optional: serve this UI build instead of the embedded one
}

func New(opt Options) (*Server, error) {
	s := &Server{
		Hostname: opt.Hostname,
		baseURL:  opt.BaseURL,
		storage:  opt.Storage,
		uiFS:     opt.UI,
		oidcHTTP: &http.Client{Timeout: 15 * time.Second},
		oidcJWKS: &jwksCache{},
	}
	if s.storage == nil {
		s.storage = &LocalStorage{Dir: "artifacts", BaseURL: opt.BaseURL}
	}
	if err := s.setUpDatabase(opt.DBPath); err != nil {
		return nil, err
	}
	s.vault = vaultFromEnv(filepath.Dir(opt.DBPath))
	return s, nil
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

// Serve starts the HTTP server with the configured routes.
func (s *Server) Serve(addr string) error {
	stype := "local"
	if _, ok := s.storage.(*S3Storage); ok {
		stype = "s3"
	}
	slog.Info("starting release-hub", "addr", addr, "storage", stype)
	return http.ListenAndServe(addr, s.routes())
}
