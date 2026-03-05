package migrations

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationFilesAndLoadStatements(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "001_init.sql")
	second := filepath.Join(dir, "002_next.sql")
	other := filepath.Join(dir, "readme.txt")

	if err := os.WriteFile(first, []byte("-- comment\nCREATE TABLE x(id int);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("INSERT INTO x VALUES (1);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := migrationFiles(dir, ".sql")
	if err != nil {
		t.Fatalf("migrationFiles failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 sql files, got %d", len(files))
	}

	stmts, err := loadStatements(first)
	if err != nil {
		t.Fatalf("loadStatements failed: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
}

func TestRunDisabledNoop(t *testing.T) {
	if err := Run(context.Background(), Config{Enabled: false}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
