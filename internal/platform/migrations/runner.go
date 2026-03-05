package migrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Enabled               bool
	PostgresDSN           string
	PostgresMigrationsDir string
}

func Run(ctx context.Context, cfg Config) error {
	if !cfg.Enabled {
		return nil
	}
	if err := runPostgres(ctx, cfg.PostgresDSN, cfg.PostgresMigrationsDir); err != nil {
		return err
	}
	return nil
}

func runPostgres(ctx context.Context, dsn, dir string) error {
	if strings.TrimSpace(dsn) == "" {
		return fmt.Errorf("postgres migration failed: empty dsn")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("postgres migration connect: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres migration ping: %w", err)
	}
	var lockAcquired bool
	if err := pool.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, int64(84642731)).Scan(&lockAcquired); err != nil {
		return fmt.Errorf("postgres migration lock check: %w", err)
	}
	if !lockAcquired {
		return fmt.Errorf("postgres migration lock is already held")
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, int64(84642731))
	}()
	if _, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version TEXT PRIMARY KEY,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`); err != nil {
		return fmt.Errorf("postgres migration bootstrap: %w", err)
	}

	files, err := migrationFiles(dir, ".sql")
	if err != nil {
		return fmt.Errorf("postgres migration files: %w", err)
	}
	for _, file := range files {
		version := filepath.Base(file)
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists); err != nil {
			return fmt.Errorf("postgres migration exists check %s: %w", file, err)
		}
		if exists {
			continue
		}

		statements, err := loadStatements(file)
		if err != nil {
			return fmt.Errorf("postgres migration read %s: %w", file, err)
		}
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("postgres migration tx begin %s: %w", file, err)
		}
		for _, stmt := range statements {
			if _, err := tx.Exec(ctx, stmt); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("postgres migration exec %s: %w", file, err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("postgres migration mark applied %s: %w", file, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("postgres migration tx commit %s: %w", file, err)
		}
	}
	return nil
}

func migrationFiles(dir, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ext {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func loadStatements(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		cleaned = append(cleaned, line)
	}

	rawStatements := strings.Split(strings.Join(cleaned, "\n"), ";")
	statements := make([]string, 0, len(rawStatements))
	for _, stmt := range rawStatements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		statements = append(statements, stmt)
	}
	return statements, nil
}
