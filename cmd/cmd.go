package cmd

type Command interface {
	Name() string
	Run(*App) error
}

type Namespaced struct {
	Namespace string
	Command
}

func NewNamespaced(namespace string, cmd Command) *Namespaced {
	return &Namespaced{
		Namespace: namespace,
		Command:   cmd,
	}
}
