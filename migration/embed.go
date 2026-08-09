package migration

import (
	"bufio"
	"embed"
	"fmt"
	"sort"
	"strings"
)

type fileMigration struct {
	author string
	name   string
	up     string
	down   string
}

func (m *fileMigration) Author() string { return m.author }
func (m *fileMigration) Name() string   { return m.name }
func (m *fileMigration) Up() string     { return m.up }
func (m *fileMigration) Down() string   { return m.down }

// ExtractFromEmbedFs parses every *.sql file in files and registers
// them as migrations, ordered by filename (so name your files with a
// sortable prefix, e.g. a timestamp: 20260809153000_author_name.sql).
func ExtractFromEmbedFs(files embed.FS) ([]Migration, error) {
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
	sort.Strings(names) // filename prefix drives ordering

	migrations := make([]Migration, 0, len(names))
	for _, fname := range names {
		b, err := files.ReadFile(fname)
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", fname, err)
		}

		m, err := parseMigrationFile(fname, string(b))
		if err != nil {
			return nil, fmt.Errorf("parse migration %q: %w", fname, err)
		}
		migrations = append(migrations, m)
	}

	return migrations, nil
}

// parseMigrationFile splits a migration file's content on "-- up" and
// "-- down" markers (case-insensitive, must be alone on their own line).
// author/name are derived from the filename: <prefix>_<author>_<name>.sql
func parseMigrationFile(filename, content string) (*fileMigration, error) {
	author, _, err := splitFilename(filename)
	if err != nil {
		return nil, err
	}

	up, down, err := splitUpDown(content)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}

	return &fileMigration{
		author: author,
		name:   strings.TrimSuffix(filename, ".sql"), // full sortable name incl. prefix
		up:     up,
		down:   down,
	}, nil
}

func splitFilename(filename string) (author, name string, err error) {
	base := strings.TrimSuffix(filename, ".sql")
	parts := strings.SplitN(base, "_", 3)
	if len(parts) != 3 {
		return "", "", fmt.Errorf("expected <prefix>_<author>_<name>.sql, got %q", filename)
	}
	return parts[1], parts[2], nil
}

func splitUpDown(content string) (up, down string, err error) {
	const (
		markerUp   = "-- up"
		markerDown = "-- down"
	)

	var (
		buf     strings.Builder
		section string // "" | "up" | "down"
	)

	scanner := bufio.NewScanner(strings.NewReader(content))
	flush := func() {
		switch section {
		case "up":
			up = strings.TrimSpace(buf.String())
		case "down":
			down = strings.TrimSpace(buf.String())
		}
		buf.Reset()
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.ToLower(strings.TrimSpace(line))

		switch trimmed {
		case markerUp:
			flush()
			section = "up"
			continue
		case markerDown:
			flush()
			section = "down"
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	flush()

	if err := scanner.Err(); err != nil {
		return "", "", err
	}
	if up == "" {
		return "", "", fmt.Errorf("missing %q section", markerUp)
	}
	if down == "" {
		return "", "", fmt.Errorf("missing %q section", markerDown)
	}
	return up, down, nil
}
