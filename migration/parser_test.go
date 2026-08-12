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
			want:    []string{"create table a (id int);", "create table b (id int);"},
		},
		{
			name:    "multiple statements on same line",
			section: "select 1; select 2;",
			want:    []string{"select 1;", "select 2;"},
		},
		{
			name:    "trailing statement without semicolon after one with",
			section: "select 1;\nselect 2",
			want:    []string{"select 1;", "select 2"},
		},
		{
			name:    "empty section",
			section: "",
			want:    nil,
		},
		{
			name:    "whitespace only section",
			section: "\n\n  \n",
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
				"create function f() returns trigger as $$\nbegin\n  new.x := 1;\n  return new;\nend;\n$$ language plpgsql;",
			},
		},
		{
			name: "statement block markers are case-insensitive and trimmed",
			section: "  -- +MIGRATE StatementBegin  \n" +
				"do something;\n" +
				"  -- +migrate STATEMENTEND\n",
			want: []string{"do something;"},
		},
		{
			name: "statement outside block after a block",
			section: "-- +migrate StatementBegin\n" +
				"begin; end;\n" +
				"-- +migrate StatementEnd\n" +
				"select 1;",
			want: []string{"begin; end;", "select 1;"},
		},
		{
			name: "block with no trailing content still flushed",
			section: "-- +migrate StatementBegin\n" +
				"begin;\n" +
				"end;\n" +
				"-- +migrate StatementEnd",
			want: []string{"begin;\nend;"},
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
	t.Run("basic up and down", func(t *testing.T) {
		content := "-- +migrate up\n" +
			"create table a (id int);\n" +
			"-- +migrate down\n" +
			"drop table a;\n"

		up, down, err := splitUpDown(content)
		require.NoError(t, err)
		assert.Equal(t, []string{"create table a (id int);"}, up)
		assert.Equal(t, []string{"drop table a;"}, down)
	})

	t.Run("markers are case-insensitive and trimmed", func(t *testing.T) {
		content := "  -- +MIGRATE UP  \n" +
			"select 1;\n" +
			"  -- +Migrate Down\n" +
			"select 2;\n"

		up, down, err := splitUpDown(content)
		require.NoError(t, err)
		assert.Equal(t, []string{"select 1;"}, up)
		assert.Equal(t, []string{"select 2;"}, down)
	})

	t.Run("multiple statements per section", func(t *testing.T) {
		content := "-- +migrate up\n" +
			"create table a (id int);\n" +
			"create table b (id int);\n" +
			"-- +migrate down\n" +
			"drop table b;\n" +
			"drop table a;\n"

		up, down, err := splitUpDown(content)
		require.NoError(t, err)
		assert.Equal(t, []string{"create table a (id int);", "create table b (id int);"}, up)
		assert.Equal(t, []string{"drop table b;", "drop table a;"}, down)
	})

	t.Run("statement block spanning multiple lines inside up", func(t *testing.T) {
		content := "-- +migrate up\n" +
			"-- +migrate StatementBegin\n" +
			"create function f() returns trigger as $$\n" +
			"begin\n" +
			"  return new;\n" +
			"end;\n" +
			"$$ language plpgsql;\n" +
			"-- +migrate StatementEnd\n" +
			"-- +migrate down\n" +
			"drop function f();\n"

		up, down, err := splitUpDown(content)
		require.NoError(t, err)
		require.Len(t, up, 1)
		assert.Contains(t, up[0], "begin\n  return new;\nend;")
		assert.Equal(t, []string{"drop function f();"}, down)
	})

	t.Run("missing up section", func(t *testing.T) {
		content := "-- +migrate down\ndrop table a;\n"

		up, down, err := splitUpDown(content)
		require.Error(t, err)
		assert.Nil(t, up)
		assert.Nil(t, down)
		assert.ErrorContains(t, err, "-- +migrate up")
	})

	t.Run("missing down section", func(t *testing.T) {
		content := "-- +migrate up\ncreate table a (id int);\n"

		up, down, err := splitUpDown(content)
		require.Error(t, err)
		assert.Nil(t, up)
		assert.Nil(t, down)
		assert.ErrorContains(t, err, "-- +migrate down")
	})

	t.Run("empty content", func(t *testing.T) {
		up, down, err := splitUpDown("")
		require.Error(t, err)
		assert.Nil(t, up)
		assert.Nil(t, down)
	})

	t.Run("down before up", func(t *testing.T) {
		content := "-- +migrate down\ndrop table a;\n-- +migrate up\ncreate table a (id int);\n"

		up, down, err := splitUpDown(content)
		require.NoError(t, err)
		assert.Equal(t, []string{"create table a (id int);"}, up)
		assert.Equal(t, []string{"drop table a;"}, down)
	})
}

func TestParseRawMigration(t *testing.T) {
	t.Run("valid migration", func(t *testing.T) {
		content := "-- +migrate up\n" +
			"create table a (id int);\n" +
			"-- +migrate down\n" +
			"drop table a;\n"

		m, err := parseRawMigration("000001_create_a", content)
		require.NoError(t, err)
		require.NotNil(t, m)
		assert.Equal(t, "000001_create_a", m.Name)
		assert.Equal(t, []string{"create table a (id int);"}, m.Up)
		assert.Equal(t, []string{"drop table a;"}, m.Down)
	})

	t.Run("invalid migration wraps name in error", func(t *testing.T) {
		content := "-- +migrate up\ncreate table a (id int);\n"

		m, err := parseRawMigration("000001_bad", content)
		require.Error(t, err)
		assert.Nil(t, m)
		assert.ErrorContains(t, err, "000001_bad")
	})
}
