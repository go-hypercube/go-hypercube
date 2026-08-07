package config

import (
	"io"
	"os"

	"github.com/pelletier/go-toml/v2"
)

type tomlConfig struct {
	data map[string]any
}

func NewTomlConfig(filePath string) Config {
	b, err := os.ReadFile(filePath)
	if err != nil {
		panic(err)
	}
	return NewTomlConfigFromString(string(b))
}

func NewTomlConfigFromString(content string) Config {
	var data map[string]any
	if err := toml.Unmarshal([]byte(content), &data); err != nil {
		panic(err)
	}
	return &tomlConfig{data: data}
}

func NewTomlConfigFromReader(r io.Reader) Config {
	var data map[string]any
	if err := toml.NewDecoder(r).Decode(&data); err != nil {
		panic(err)
	}
	return &tomlConfig{data: data}
}

func (c *tomlConfig) ReadInt(key string) int64 {
	return toInt64(c.data[key])
}

func (c *tomlConfig) ReadString(key string) string {
	return toString(c.data[key])
}

func (c *tomlConfig) ReadFloat(key string) float64 {
	return toFloat64(c.data[key])
}
