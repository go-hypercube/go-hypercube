package migration

type Migration interface {
	Name() string // Will order the Migration by it
	Up() string
	Down() string
}
