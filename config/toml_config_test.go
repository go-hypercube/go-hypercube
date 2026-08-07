package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTomlConfig(t *testing.T) {
	content := `
name = "hypercube"
port = 8080
ratio = 3.14
`
	path := filepath.Join(t.TempDir(), "config.toml")
	err := os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)

	cfg, err := NewTomlConfig(path)
	require.NotNil(t, cfg)
	assert.NoError(t, err)

	assert.Equal(t, "hypercube", cfg.ReadString("name"))
	assert.Equal(t, int64(8080), cfg.ReadInt("port"))
	assert.Equal(t, 3.14, cfg.ReadFloat("ratio"))
	assert.Equal(t, "", cfg.ReadString("missing"))
	assert.Equal(t, int64(0), cfg.ReadInt("missing"))
}

func TestTomlConfigFromString(t *testing.T) {
	content := `
name = "hypercube"
port = 8080
ratio = 3.14
`
	cfg, err := NewTomlConfigFromString(content)

	assert.NoError(t, err)
	assert.Equal(t, "hypercube", cfg.ReadString("name"))
	assert.Equal(t, int64(8080), cfg.ReadInt("port"))
	assert.Equal(t, 3.14, cfg.ReadFloat("ratio"))
	assert.Equal(t, "", cfg.ReadString("missing"))
	assert.Equal(t, int64(0), cfg.ReadInt("missing"))
}

func TestNewTomlConfigFromReader(t *testing.T) {
	content := `
name = "hypercube"
port = 8080
ratio = 3.14
`
	cfg, err := NewTomlConfigFromReader(strings.NewReader(content))

	assert.NoError(t, err)
	assert.Equal(t, "hypercube", cfg.ReadString("name"))
	assert.Equal(t, int64(8080), cfg.ReadInt("port"))
	assert.Equal(t, 3.14, cfg.ReadFloat("ratio"))
	assert.Equal(t, "", cfg.ReadString("missing"))
	assert.Equal(t, int64(0), cfg.ReadInt("missing"))
}

func TestNewTomlConfigFromReader_InvalidContent(t *testing.T) {
	cfg, err := NewTomlConfigFromReader(strings.NewReader(`not = [valid toml`))
	assert.Error(t, err)
	assert.Nil(t, cfg)
}
