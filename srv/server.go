package srv

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"srv.exe.dev/db"
)

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	baseURL      string  // public base URL for links
	storage      Storage // local FS or S3 (see storage.go)

	// embeddedTemplates: serve go:embed'ded templates instead of
	// TemplatesDir (set when the on-disk dir is absent — e.g. Docker).
	embeddedTemplates bool
}

// Options configures a Server at construction time (listmonk-style config
// injection instead of exported mutable fields).
type Options struct {
	DBPath   string  // sqlite database path
	Hostname string  // for display/debugging
	BaseURL  string  // public base URL for manifest/download links
	Storage  Storage // local FS or S3 (see storage.go); required
}

func New(opt Options) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	s := &Server{
		Hostname: opt.Hostname,
		baseURL:  opt.BaseURL,
		storage:  opt.Storage,
		// Prefer the on-disk copy next to the source (dev checkout); fall back
		// to the go:embed copies baked into the binary. The fallback is what
		// makes the Docker image self-contained: runtime.Caller returns the
		// BUILD-time path (/src/srv/...), which doesn't exist in the runtime
		// image — only the binary is copied over.
		TemplatesDir: filepath.Join(baseDir, "templates"),
	}
	if s.storage == nil {
		s.storage = &LocalStorage{Dir: "artifacts", BaseURL: opt.BaseURL}
	}
	if _, err := os.Stat(s.TemplatesDir); err != nil {
		s.embeddedTemplates = true
	}
	if err := s.setUpDatabase(opt.DBPath); err != nil {
		return nil, err
	}
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
