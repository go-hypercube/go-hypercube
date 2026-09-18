package job

import (
	"github.com/go-hypercube/go-hypercube/namespaced"
)

type Job interface {
	Name() string
	Handle(app *App, payload []byte) error
}

func NewNamespaced(namespace string, j Job) *namespaced.Namespaced[Job] {
	return &namespaced.Namespaced[Job]{Namespace: namespace, Item: j}
}
