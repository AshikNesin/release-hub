package db

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:generate go tool github.com/sqlc-dev/sqlc/cmd/sqlc generate

//go:embed migrations/*.sql
var migrationFS embed.FS

// Open opens an sqlite database and prepares pragmas suitable for a small web app.
//
// The pragmas ride on the DSN: modernc.org/sqlite applies query parameters
// per connection, whereas db.Exec would only configure whichever pooled
// connection happened to run it — and foreign_keys (the ON DELETE CASCADE
// behind app deletion) is per-connection in sqlite.
func Open(path string) (*sql.DB, error) {
	dsn := path
	if i := strings.Index(dsn, "?"); i >= 0 { // caller-supplied params win
		dsn = strings.TrimSuffix(dsn[:i], "&") + "&"
	} else {
		dsn += "?"
	}
	dsn += "_pragma=foreign_keys(1)&_pragma=busy_timeout(1000)&_pragma=journal_mode(wal)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// RunMigrations executes database migrations in numeric order (NNN-*.sql),
// similar in spirit to exed's exedb.RunMigrations.
func RunMigrations(db *sql.DB) error {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	var migrations []string
	pat := regexp.MustCompile(`^(\d{3})-.*\.sql$`)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if pat.MatchString(name) {
			migrations = append(migrations, name)
		}
	}
	sort.Strings(migrations)

	executed := make(map[int]bool)
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='migrations'").Scan(&tableName)
	switch {
	case err == nil:
		rows, err := db.Query("SELECT migration_number FROM migrations")
		if err != nil {
			return fmt.Errorf("query executed migrations: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var n int
			if err := rows.Scan(&n); err != nil {
				return fmt.Errorf("scan migration number: %w", err)
			}
			executed[n] = true
		}
	case errors.Is(err, sql.ErrNoRows):
		slog.Info("db: migrations table not found; running all migrations")
	default:
		return fmt.Errorf("check migrations table: %w", err)
	}

	for _, m := range migrations {
		match := pat.FindStringSubmatch(m)
		if len(match) != 2 {
			return fmt.Errorf("invalid migration filename: %s", m)
		}
		n, err := strconv.Atoi(match[1])
		if err != nil {
			return fmt.Errorf("parse migration number %s: %w", m, err)
		}
		if executed[n] {
			continue
		}
		if err := executeMigration(db, m); err != nil {
			return fmt.Errorf("execute %s: %w", m, err)
		}
		slog.Info("db: applied migration", "file", m, "number", n)
	}
	return nil
}

func executeMigration(db *sql.DB, filename string) error {
	content, err := migrationFS.ReadFile("migrations/" + filename)
	if err != nil {
		return fmt.Errorf("read %s: %w", filename, err)
	}
	if _, err := db.Exec(string(content)); err != nil {
		return fmt.Errorf("exec %s: %w", filename, err)
	}
	return nil
}
