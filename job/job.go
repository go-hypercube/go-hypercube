package job

import (
	"errors"

	"github.com/go-hypercube/go-hypercube/namespaced"
)

// ErrNilJob indicates that a nil job was supplied for registration.
var ErrNilJob = errors.New("job must not be nil")

// ErrEmptyName indicates that a registered job has no name.
var ErrEmptyName = errors.New("job name must not be empty")

type Job interface {
	Name() string
	Handle(app *App, payload []byte) error
}

func NewNamespaced(namespace string, j Job) *namespaced.Namespaced[Job] {
	return &namespaced.Namespaced[Job]{Namespace: namespace, Item: j}
}
