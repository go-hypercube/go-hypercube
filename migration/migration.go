package migration

type Migration struct {
	Name string
	Up   []string
	Down []string
}

func ParseRawMigration(migrationName, content string) (*Migration, error) {
	m, err := parseRawMigration(migrationName, content)
	if err != nil {
		return nil, err
	}
	return m, nil
}

type Namespaced struct {
	Namespace  string
	*Migration
}

func NewNamespaced(namespace string, migration *Migration) *Namespaced {
	return &Namespaced{
		Namespace:  namespace,
		Migration: migration,
	}
}
