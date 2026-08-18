package migration

import (
	"bufio"
	"fmt"
	"strings"
)

// ParseRawMigration parses the contents of a migration file into a Migration.
//
// A migration must contain two sections:
//
//	-- +migrate up
//
//	<SQL statements to apply the migration>
//
//	-- +migrate down
//
//	<SQL statements to revert the migration>
//
// The "up" and "down" markers are case-insensitive and must appear alone on
// their own line. Leading and trailing whitespace around the markers is
// ignored.
//
// Each section is further split into individual SQL statements using
// semicolons. For SQL that contains semicolons internally, such as PostgreSQL
// functions, procedures, or triggers, the statements must be wrapped with:
//
//	-- +migrate StatementBegin
//	...
//	-- +migrate StatementEnd
//
// Everything between StatementBegin and StatementEnd is kept as a single
// statement, regardless of the number of semicolons it contains.
//
// For example:
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
// A function containing internal semicolons should instead be written as:
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
// Returns a Migration containing the migration name and the parsed "up" and
// "down" SQL statements.
//
// Returns an error when the migration does not contain a valid "up" or "down"
// section, or when the migration content cannot be parsed.
func ParseRawMigration(migrationName, content string) (*Migration, error) {
	up, down, err := splitUpDown(content)
	if err != nil {
		return nil, fmt.Errorf("parse migration: %w", err)
	}
	return &Migration{
		Name: migrationName,
		Up:   up,
		Down: down,
	}, nil
}

func splitUpDown(content string) (up, down []string, err error) {
	const (
		markerUp   = "-- +migrate up"
		markerDown = "-- +migrate down"
	)
	var (
		buf     strings.Builder
		section string // "" | "up" | "down"
	)
	scanner := bufio.NewScanner(strings.NewReader(content))
	flush := func() {
		switch section {
		case "up":
			up = splitStatements(buf.String())
		case "down":
			down = splitStatements(buf.String())
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
		return nil, nil, err
	}
	if len(up) == 0 {
		return nil, nil, fmt.Errorf("missing %q section", markerUp)
	}
	if len(down) == 0 {
		return nil, nil, fmt.Errorf("missing %q section", markerDown)
	}
	return up, down, nil
}

// splitStatements splits a section's raw SQL into individual statements on
// ';'. Lines between "-- +migrate StatementBegin" and "-- +migrate
// StatementEnd" (case-insensitive, alone on their own line) are kept as a
// single statement regardless of any ';' inside them — needed for function/
// trigger bodies with internal BEGIN...END; blocks.
func splitStatements(section string) []string {
	const (
		markerBegin = "-- +migrate statementbegin"
		markerEnd   = "-- +migrate statementend"
	)
	var (
		statements []string
		buf        strings.Builder
		inBlock    bool
	)
	flush := func() {
		s := strings.TrimSpace(buf.String())
		if s != "" {
			statements = append(statements, s)
		}
		buf.Reset()
	}
	scanner := bufio.NewScanner(strings.NewReader(section))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.ToLower(strings.TrimSpace(line))
		switch trimmed {
		case markerBegin:
			inBlock = true
			continue
		case markerEnd:
			inBlock = false
			flush()
			continue
		}
		if inBlock {
			buf.WriteString(line)
			buf.WriteString("\n")
			continue
		}
		rest := line
		for {
			idx := strings.Index(rest, ";")
			if idx == -1 {
				buf.WriteString(rest)
				buf.WriteString("\n")
				break
			}
			buf.WriteString(rest[:idx+1])
			flush()
			rest = rest[idx+1:]
		}
	}
	flush()
	return statements
}
