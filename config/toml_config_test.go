package config

import (
	"os"
	"path/filepath"
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

	cfg := NewTomlConfig(path)
	require.NotNil(t, cfg)

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
	cfg := NewTomlConfigFromString(content)

	assert.Equal(t, "hypercube", cfg.ReadString("name"))
	assert.Equal(t, int64(8080), cfg.ReadInt("port"))
	assert.Equal(t, 3.14, cfg.ReadFloat("ratio"))
	assert.Equal(t, "", cfg.ReadString("missing"))
	assert.Equal(t, int64(0), cfg.ReadInt("missing"))
}
