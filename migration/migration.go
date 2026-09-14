package migration

import "github.com/go-hypercube/go-hypercube/namespaced"

// Migration is a single, named database migration consisting of a set
// of "up" SQL statements to apply the change and a set of "down" SQL
// statements to revert it. Name is expected to be sortable across
// migrations within the same namespace (e.g. a timestamp or zero-padded
// sequence prefix — see migration.ExtractFromEmbedFs) so that Up/Down
// are lexicographically ordered by Name.
//
// See ParseRawMigration and ExtractFromEmbedFs for how Up/Down are
// produced from a raw *.sql migration file.
type Migration struct {
	MigrationName string
	Up            []string
	Down          []string
}

func (m *Migration) Name() string {
	return m.MigrationName
}

func NewNamespaced(namespace string, m *Migration) *namespaced.Namespaced[*Migration] {
	return &namespaced.Namespaced[*Migration]{
		Namespace: namespace,
		Item:      m,
	}
}
