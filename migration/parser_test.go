package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		name    string
		section string
		want    []string
	}{
		{
			name:    "single statement no trailing semicolon",
			section: "select 1",
			want:    []string{"select 1"},
		},
		{
			name:    "single statement with trailing semicolon",
			section: "select 1;",
			want:    []string{"select 1;"},
		},
		{
			name:    "multiple statements one per line",
			section: "create table a (id int);\ncreate table b (id int);",
			want: []string{
				"create table a (id int);",
				"create table b (id int);",
			},
		},
		{
			name:    "multiple statements on same line",
			section: "select 1; select 2;",
			want: []string{
				"select 1;",
				"select 2;",
			},
		},
		{
			name:    "trailing statement without semicolon after one with",
			section: "select 1;\nselect 2",
			want: []string{
				"select 1;",
				"select 2",
			},
		},
		{
			name:    "empty section",
			section: "",
			want:    nil,
		},
		{
			name:    "whitespace only section",
			section: "\n\n  \n\t",
			want:    nil,
		},
		{
			name: "statement block preserves internal semicolons",
			section: "-- +migrate StatementBegin\n" +
				"create function f() returns trigger as $$\n" +
				"begin\n" +
				"  new.x := 1;\n" +
				"  return new;\n" +
				"end;\n" +
				"$$ language plpgsql;\n" +
				"-- +migrate StatementEnd\n",
			want: []string{
				"create function f() returns trigger as $$\n" +
					"begin\n" +
					"  new.x := 1;\n" +
					"  return new;\n" +
					"end;\n" +
					"$$ language plpgsql;",
			},
		},
		{
			name: "statement block markers are case-insensitive and trimmed",
			section: "  -- +MIGRATE StatementBegin  \n" +
				"do something;\n" +
				"  -- +migrate STATEMENTEND\n",
			want: []string{
				"do something;",
			},
		},
		{
			name: "statement outside block after a block",
			section: "-- +migrate StatementBegin\n" +
				"begin; end;\n" +
				"-- +migrate StatementEnd\n" +
				"select 1;",
			want: []string{
				"begin; end;",
				"select 1;",
			},
		},
		{
			name: "statement before block",
			section: "select 1;\n" +
				"-- +migrate StatementBegin\n" +
				"begin;\n" +
				"  return;\n" +
				"end;\n" +
				"-- +migrate StatementEnd\n",
			want: []string{
				"select 1;",
				"begin;\n  return;\nend;",
			},
		},
		{
			name: "block with no trailing content still flushed",
			section: "-- +migrate StatementBegin\n" +
				"begin;\n" +
				"end;\n" +
				"-- +migrate StatementEnd",
			want: []string{
				"begin;\nend;",
			},
		},
		{
			name: "empty block",
			section: "-- +migrate StatementBegin\n" +
				"-- +migrate StatementEnd\n",
			want: nil,
		},
		{
			name: "multiple statement blocks",
			section: "-- +migrate StatementBegin\n" +
				"select 1;\n" +
				"select 2;\n" +
				"-- +migrate StatementEnd\n" +
				"-- +migrate StatementBegin\n" +
				"select 3;\n" +
				"select 4;\n" +
				"-- +migrate StatementEnd\n",
			want: []string{
				"select 1;\nselect 2;",
				"select 3;\nselect 4;",
			},
		},
		{
			name: "block followed by multiple normal statements",
			section: "-- +migrate StatementBegin\n" +
				"begin;\n" +
				"end;\n" +
				"-- +migrate StatementEnd\n" +
				"select 1;\n" +
				"select 2;",
			want: []string{
				"begin;\nend;",
				"select 1;",
				"select 2;",
			},
		},
		{
			// This documents the current parser behavior.
			// The parser is not SQL-aware and treats every ';' as a delimiter
			// unless it is inside a StatementBegin/StatementEnd block.
			name:    "semicolon inside string is treated as delimiter",
			section: `select 'hello;world';`,
			want: []string{
				"select 'hello;",
				"world';",
			},
		},
		{
			// Same limitation applies to SQL comments.
			name:    "semicolon inside line comment is treated as delimiter",
			section: "select 1; -- comment; with semicolon\nselect 2;",
			want: []string{
				"select 1;",
				"-- comment;",
				"with semicolon\nselect 2;",
			},
		},
		{
			// The parser does not currently validate that StatementBegin and
			// StatementEnd are balanced.
			name: "unterminated statement block",
			section: "-- +migrate StatementBegin\n" +
				"select 1;\n" +
				"select 2;",
			want: []string{
				"select 1;\nselect 2;",
			},
		},
		{
			// The parser currently accepts StatementEnd without a matching
			// StatementBegin.
			name: "statement end without begin",
			section: "select 1;\n" +
				"-- +migrate StatementEnd\n" +
				"select 2;",
			want: []string{
				"select 1;",
				"select 2;",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitStatements(tt.section)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSplitUpDown(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantUp  []string
		wantDn  []string
		wantErr string
	}{
		{
			name: "basic up and down",
			content: "-- +migrate up\n" +
				"create table a (id int);\n" +
				"-- +migrate down\n" +
				"drop table a;\n",
			wantUp: []string{
				"create table a (id int);",
			},
			wantDn: []string{
				"drop table a;",
			},
		},
		{
			name: "markers are case-insensitive and trimmed",
			content: "  -- +MIGRATE UP  \n" +
				"select 1;\n" +
				"  -- +Migrate Down\n" +
				"select 2;\n",
			wantUp: []string{
				"select 1;",
			},
			wantDn: []string{
				"select 2;",
			},
		},
		{
			name: "multiple statements per section",
			content: "-- +migrate up\n" +
				"create table a (id int);\n" +
				"create table b (id int);\n" +
				"-- +migrate down\n" +
				"drop table b;\n" +
				"drop table a;\n",
			wantUp: []string{
				"create table a (id int);",
				"create table b (id int);",
			},
			wantDn: []string{
				"drop table b;",
				"drop table a;",
			},
		},
		{
			name: "statement block inside up",
			content: "-- +migrate up\n" +
				"-- +migrate StatementBegin\n" +
				"create function f() returns trigger as $$\n" +
				"begin\n" +
				"  return new;\n" +
				"end;\n" +
				"$$ language plpgsql;\n" +
				"-- +migrate StatementEnd\n" +
				"-- +migrate down\n" +
				"drop function f();\n",
			wantUp: []string{
				"create function f() returns trigger as $$\n" +
					"begin\n" +
					"  return new;\n" +
					"end;\n" +
					"$$ language plpgsql;",
			},
			wantDn: []string{
				"drop function f();",
			},
		},
		{
			name: "statement block inside down",
			content: "-- +migrate up\n" +
				"create table a (id int);\n" +
				"-- +migrate down\n" +
				"-- +migrate StatementBegin\n" +
				"drop table a;\n" +
				"select 1;\n" +
				"-- +migrate StatementEnd\n",
			wantUp: []string{
				"create table a (id int);",
			},
			wantDn: []string{
				"drop table a;\nselect 1;",
			},
		},
		{
			name: "down before up",
			content: "-- +migrate down\n" +
				"drop table a;\n" +
				"-- +migrate up\n" +
				"create table a (id int);\n",
			wantUp: []string{
				"create table a (id int);",
			},
			wantDn: []string{
				"drop table a;",
			},
		},
		{
			name:    "empty content",
			content: "",
			wantErr: `missing "-- +migrate up" section`,
		},
		{
			name: "missing up section",
			content: "-- +migrate down\n" +
				"drop table a;\n",
			wantErr: `missing "-- +migrate up" section`,
		},
		{
			name: "missing down section",
			content: "-- +migrate up\n" +
				"create table a (id int);\n",
			wantErr: `missing "-- +migrate down" section`,
		},
		{
			name: "empty up section",
			content: "-- +migrate up\n" +
				"-- +migrate down\n" +
				"drop table a;\n",
			wantErr: `missing "-- +migrate up" section`,
		},
		{
			name: "empty down section",
			content: "-- +migrate up\n" +
				"create table a (id int);\n" +
				"-- +migrate down\n",
			wantErr: `missing "-- +migrate down" section`,
		},
		{
			name: "whitespace before up marker",
			content: "\n\n" +
				"   -- +migrate up   \n" +
				"create table a (id int);\n" +
				"-- +migrate down\n" +
				"drop table a;\n",
			wantUp: []string{
				"create table a (id int);",
			},
			wantDn: []string{
				"drop table a;",
			},
		},
		{
			// Content before the first marker is currently ignored because
			// splitUpDown only collects content while inside a section.
			name: "content before up is ignored",
			content: "ignored content;\n" +
				"-- +migrate up\n" +
				"create table a (id int);\n" +
				"-- +migrate down\n" +
				"drop table a;\n",
			wantUp: []string{
				"create table a (id int);",
			},
			wantDn: []string{
				"drop table a;",
			},
		},
		{
			name: "content after down belongs to down section",
			content: "-- +migrate up\n" +
				"create table a (id int);\n" +
				"-- +migrate down\n" +
				"drop table a;\n" +
				"drop table b;\n",
			wantUp: []string{
				"create table a (id int);",
			},
			wantDn: []string{
				"drop table a;",
				"drop table b;",
			},
		},
		{
			// The current implementation overwrites the previous "up" section
			// when another up marker is encountered.
			name: "duplicate up marker keeps only last up section",
			content: "-- +migrate up\n" +
				"create table a (id int);\n" +
				"-- +migrate up\n" +
				"create table b (id int);\n" +
				"-- +migrate down\n" +
				"drop table b;\n" +
				"drop table a;\n",
			wantUp: []string{
				"create table b (id int);",
			},
			wantDn: []string{
				"drop table b;",
				"drop table a;",
			},
		},
		{
			// The current implementation overwrites the previous "down" section
			// when another down marker is encountered.
			name: "duplicate down marker keeps only last down section",
			content: "-- +migrate up\n" +
				"create table a (id int);\n" +
				"-- +migrate down\n" +
				"drop table a;\n" +
				"-- +migrate down\n" +
				"drop table b;\n",
			wantUp: []string{
				"create table a (id int);",
			},
			wantDn: []string{
				"drop table b;",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up, down, err := splitUpDown(tt.content)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Nil(t, up)
				assert.Nil(t, down)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantUp, up)
			assert.Equal(t, tt.wantDn, down)
		})
	}
}

func TestParseRawMigration(t *testing.T) {
	t.Run("valid migration", func(t *testing.T) {
		content := "-- +migrate up\n" +
			"create table a (id int);\n" +
			"-- +migrate down\n" +
			"drop table a;\n"

		m, err := ParseRawMigration("000001_create_a.sql", content)

		require.NoError(t, err)
		require.NotNil(t, m)

		assert.Equal(t, "000001_create_a.sql", m.Name)
		assert.Equal(t, []string{
			"create table a (id int);",
		}, m.Up)
		assert.Equal(t, []string{
			"drop table a;",
		}, m.Down)
	})

	t.Run("valid migration with statement blocks", func(t *testing.T) {
		content := "-- +migrate up\n" +
			"-- +migrate StatementBegin\n" +
			"create function f() returns void as $$\n" +
			"begin\n" +
			"  perform 1;\n" +
			"  perform 2;\n" +
			"end;\n" +
			"$$ language plpgsql;\n" +
			"-- +migrate StatementEnd\n" +
			"-- +migrate down\n" +
			"drop function f();\n"

		m, err := ParseRawMigration("000002_create_function.sql", content)

		require.NoError(t, err)
		require.NotNil(t, m)

		assert.Equal(t, "000002_create_function.sql", m.Name)

		require.Len(t, m.Up, 1)
		assert.Contains(t, m.Up[0], "perform 1;")
		assert.Contains(t, m.Up[0], "perform 2;")

		assert.Equal(t, []string{
			"drop function f();",
		}, m.Down)
	})

	t.Run("invalid migration wraps parser error with migration name", func(t *testing.T) {
		content := "-- +migrate up\n" +
			"create table a (id int);\n"

		m, err := ParseRawMigration("000001_bad.sql", content)

		require.Error(t, err)
		assert.Nil(t, m)

		assert.ErrorContains(t, err, "000001_bad.sql")
		assert.ErrorContains(t, err, `missing "-- +migrate down" section`)
	})
}
