package db

import (
	"path/filepath"
	"testing"
)

// Every pooled connection must get the DSN pragmas: foreign_keys is
// per-connection in sqlite, and the ON DELETE CASCADE behind app deletion
// silently no-ops on a connection without it.
func TestOpenAppliesPragmasToEveryConnection(t *testing.T) {
	d := t.TempDir()

	db, err := Open(filepath.Join(d, "pragma.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE p (id INTEGER PRIMARY KEY);
CREATE TABLE c (pid INTEGER NOT NULL REFERENCES p(id) ON DELETE CASCADE);
INSERT INTO p (id) VALUES (1);`); err != nil {
		t.Fatal(err)
	}

	// Hold one connection open, then check a *second* connection.
	rows, err := db.Query(`SELECT 1`)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("no rows")
	}
	defer rows.Close()

	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Fatalf("second connection has foreign_keys=%d; cascades would silently no-op", fk)
	}
}
