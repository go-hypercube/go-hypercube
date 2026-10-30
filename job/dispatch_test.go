package job

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDispatchUnavailable(t *testing.T) {
	app := NewAppForJob(&Options{})
	require.ErrorIs(t, app.Dispatch(t.Context(), "follow-up", nil, DispatchConfig{}), ErrDispatchUnavailable)
}
