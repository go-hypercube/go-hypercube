package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonConfig(t *testing.T) {
	content := `{
		"name": "hypercube",
		"port": 8080,
		"ratio": 3.14
	}`
	path := filepath.Join(t.TempDir(), "config.json")
	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)

	cfg, err := NewJsonConfig(path)
	require.NotNil(t, cfg)
	assert.NoError(t, err)

	assert.Equal(t, "hypercube", cfg.ReadString("name"))
	assert.Equal(t, int64(8080), cfg.ReadInt("port"))
	assert.Equal(t, 3.14, cfg.ReadFloat("ratio"))
	assert.Equal(t, "", cfg.ReadString("missing"))
	assert.Equal(t, int64(0), cfg.ReadInt("missing"))
}

func TestJsonConfigFromString(t *testing.T) {
	content := `{
		"name": "hypercube",
		"port": 8080,
		"ratio": 3.14
	}`
	cfg, err := NewJsonConfigFromString(content)

	assert.NoError(t, err)
	assert.Equal(t, "hypercube", cfg.ReadString("name"))
	assert.Equal(t, int64(8080), cfg.ReadInt("port"))
	assert.Equal(t, 3.14, cfg.ReadFloat("ratio"))
	assert.Equal(t, "", cfg.ReadString("missing"))
	assert.Equal(t, int64(0), cfg.ReadInt("missing"))
}

func TestNewJsonConfigFromReader(t *testing.T) {
	content := `{
		"name": "hypercube",
		"port": 8080,
		"ratio": 3.14
	}`

	cfg, err := NewJsonConfigFromReader(strings.NewReader(content))

	assert.NoError(t, err)
	assert.Equal(t, "hypercube", cfg.ReadString("name"))
	assert.Equal(t, int64(8080), cfg.ReadInt("port"))
	assert.Equal(t, 3.14, cfg.ReadFloat("ratio"))
	assert.Equal(t, "", cfg.ReadString("missing"))
	assert.Equal(t, int64(0), cfg.ReadInt("missing"))
}

func TestNewJsonConfigFromReader_InvalidContent(t *testing.T) {
	cfg, err := NewJsonConfigFromReader(strings.NewReader(`{invalid json`))
	assert.Error(t, err)
	assert.Nil(t, cfg)
}
