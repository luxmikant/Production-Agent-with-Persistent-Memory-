package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run applies every *.sql file in dir, in filename order, executing each
// statement individually so it works with any CockroachDB cluster regardless
// of how the connecting client handles multi-statement batches.
func Run(ctx context.Context, url, dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return fmt.Errorf("no migration files found in %s", dir)
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}

	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		stmts := Split(b)
		fmt.Printf("== %s (%d statements)\n", filepath.Base(f), len(stmts))
		for i, s := range stmts {
			if _, err := pool.Exec(ctx, s); err != nil {
				return fmt.Errorf("%s stmt %d: %w\n%s", filepath.Base(f), i+1, err, s)
			}
			fmt.Printf("   ok stmt %d\n", i+1)
		}
	}
	return nil
}

// Split splits a SQL script into individual statements.
// A statement ends at a line whose trimmed text ends with ';'.
// Comment lines and blank lines are skipped.
func Split(script []byte) []string {
	var out []string
	var cur strings.Builder
	for _, raw := range strings.Split(string(script), "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		cur.WriteString(raw)
		cur.WriteByte('\n')
		if strings.HasSuffix(trimmed, ";") {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}
