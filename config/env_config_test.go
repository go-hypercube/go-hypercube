package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvConfig(t *testing.T) {
	content := `NAME=hypercube
PORT=8080
RATIO=3.14
`
	path := filepath.Join(t.TempDir(), ".env")
	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)

	cfg := NewEnvConfig(path)
	require.NotNil(t, cfg)

	assert.Equal(t, "hypercube", cfg.ReadString("NAME"))
	assert.Equal(t, int64(8080), cfg.ReadInt("PORT"))
	assert.Equal(t, 3.14, cfg.ReadFloat("RATIO"))
	assert.Equal(t, "", cfg.ReadString("MISSING"))
	assert.Equal(t, int64(0), cfg.ReadInt("MISSING"))
}

func TestEnvConfigFromString(t *testing.T) {
	content := `NAME=hypercube
PORT=8080
RATIO=3.14
`
	cfg := NewEnvConfigFromString(content)

	assert.Equal(t, "hypercube", cfg.ReadString("NAME"))
	assert.Equal(t, int64(8080), cfg.ReadInt("PORT"))
	assert.Equal(t, 3.14, cfg.ReadFloat("RATIO"))
	assert.Equal(t, "", cfg.ReadString("MISSING"))
	assert.Equal(t, int64(0), cfg.ReadInt("MISSING"))
}

func TestNewEnvConfigFromReader(t *testing.T) {
	content := `NAME=hypercube
PORT=8080
RATIO=3.14
`
	cfg := NewEnvConfigFromReader(strings.NewReader(content))

	assert.Equal(t, "hypercube", cfg.ReadString("NAME"))
	assert.Equal(t, int64(8080), cfg.ReadInt("PORT"))
	assert.Equal(t, 3.14, cfg.ReadFloat("RATIO"))
	assert.Equal(t, "", cfg.ReadString("MISSING"))
	assert.Equal(t, int64(0), cfg.ReadInt("MISSING"))
}

func TestNewEnvConfigFromReader_InvalidContent(t *testing.T) {
	assert.Panics(t, func() {
		NewEnvConfigFromReader(strings.NewReader(`this is not valid env=format="`))
	})
}
