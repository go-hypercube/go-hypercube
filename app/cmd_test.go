package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- registerCommandForNamespace override behavior ----

func TestRegisterCommandForNamespace_OverridesExistingByName(t *testing.T) {
	t.Run("re-registering the same (namespace, name) replaces in place", func(t *testing.T) {
		app := &App{}
		original := &fakeCommand{name: "seed"}
		replacement := &fakeCommand{name: "seed"}

		require.NoError(t, app.registerCommandForNamespace("auth", original))
		require.NoError(t, app.registerCommandForNamespace("auth", replacement))

		got := app.cmds
		require.Len(t, got, 1)
		assert.Same(t, replacement, got[0].Command)
	})

	t.Run("replacement preserves original position", func(t *testing.T) {
		app := &App{}
		c1 := &fakeCommand{name: "a"}
		c2 := &fakeCommand{name: "b"}
		c3 := &fakeCommand{name: "c"}
		require.NoError(t, app.registerCommandForNamespace("auth", c1, c2, c3))

		replacement := &fakeCommand{name: "b"}
		require.NoError(t, app.registerCommandForNamespace("auth", replacement))

		got := app.cmds
		require.Len(t, got, 3)
		assert.Same(t, c1, got[0].Command)
		assert.Same(t, replacement, got[1].Command)
		assert.Same(t, c3, got[2].Command)
	})

	t.Run("same name in a different namespace is unaffected", func(t *testing.T) {
		app := &App{}
		authCmd := &fakeCommand{name: "init"}
		billingCmd := &fakeCommand{name: "init"}

		require.NoError(t, app.registerCommandForNamespace("auth", authCmd))
		require.NoError(t, app.registerCommandForNamespace("billing", billingCmd))

		got := app.cmds
		require.Len(t, got, 2)
	})
}
