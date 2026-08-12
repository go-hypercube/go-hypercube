package migration

import (
	"embed"
	"fmt"
	"strings"
)

// ExtractFromEmbedFs parses every *.sql file in files and registers
// them as migrations, ordered by filename (so name your files with a
// sortable prefix, e.g. a timestamp: 20260809153000_name.sql.
func ExtractFromEmbedFs(files embed.FS) ([]*Migration, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read migration dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}

	migrations := make([]*Migration, 0, len(names))
	for _, fname := range names {
		b, err := files.ReadFile(fname)
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", fname, err)
		}

		m, err := parseRawMigration(fname, string(b))
		if err != nil {
			return nil, fmt.Errorf("parse migration %q: %w", fname, err)
		}
		migrations = append(migrations, m)
	}

	return migrations, nil
}
