package migration

import (
	"embed"
	"fmt"
	"slices"
	"strings"
)

// ExtractFromEmbedFs parses every *.sql file in the root of the given embed.FS.
//
// Migration files are processed in lexicographical order by file name. Because
// of this, migration file names should use a sortable format, such as:
//
//	20260809153000_create_users.sql
//	20260809154000_create_posts.sql
//
// or:
//
//	000001_create_users.sql
//	000002_create_posts.sql
//
// Only *.sql files directly in the root of the embed.FS are considered.
// Directories and files with other extensions are ignored.
//
// Each migration file must contain both an "up" and a "down" section using the
// following markers:
//
//	-- +migrate up
//
//	<SQL statements>
//
//	-- +migrate down
//
//	<SQL statements>
//
// The section markers are case-insensitive and must appear alone on their own
// line. Leading and trailing whitespace is allowed.
//
// SQL statements inside a section are normally separated by semicolons:
//
//	-- +migrate up
//
//	CREATE TABLE users (
//	    id BIGSERIAL PRIMARY KEY,
//	    name TEXT NOT NULL
//	);
//
//	CREATE INDEX users_name_idx ON users(name);
//
//	-- +migrate down
//
//	DROP INDEX users_name_idx;
//	DROP TABLE users;
//
// Each semicolon normally terminates one statement. Therefore, SQL that
// contains internal semicolons, such as PostgreSQL functions, procedures, or
// triggers, must be wrapped between:
//
//	-- +migrate StatementBegin
//	...
//	-- +migrate StatementEnd
//
// Everything between StatementBegin and StatementEnd is treated as one
// statement regardless of how many semicolons it contains.
//
// For example:
//
//	-- +migrate up
//
//	-- +migrate StatementBegin
//	CREATE FUNCTION update_timestamp()
//	RETURNS TRIGGER AS $$
//	BEGIN
//	    NEW.updated_at = NOW();
//	    RETURN NEW;
//	END;
//	$$ LANGUAGE plpgsql;
//	-- +migrate StatementEnd
//
//	-- +migrate down
//
//	-- +migrate StatementBegin
//	DROP FUNCTION update_timestamp();
//	-- +migrate StatementEnd
//
// StatementBegin and StatementEnd are also case-insensitive and must appear
// alone on their own line.
//
// The order returned by this function is deterministic and matches the
// lexicographical order of the migration file names. The ordering is therefore
// significant when migrations are executed sequentially.
//
// Returns:
//
//   - A slice of *Migration, one per *.sql file, sorted lexicographically by
//     file name.
//   - An error if the migration directory cannot be read, a migration file
//     cannot be read, or a migration file is invalid.
//
// A migration is invalid when its required "up" or "down" section is missing
// or does not contain any SQL statements.
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

	// Sort names for deterministic order.
	slices.Sort(names)

	migrations := make([]*Migration, 0, len(names))

	for _, fname := range names {
		b, err := files.ReadFile(fname)
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", fname, err)
		}

		m, err := ParseRawMigration(fname, string(b))
		if err != nil {
			return nil, fmt.Errorf("parse migration %q: %w", fname, err)
		}

		migrations = append(migrations, m)
	}

	return migrations, nil
}
