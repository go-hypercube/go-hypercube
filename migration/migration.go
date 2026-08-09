package migration

type Migration interface {
	Author() string
	Name() string // Will order the Migration by it
	Up() string
	Down() string
}
