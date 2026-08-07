package config

import (
	"encoding/json"
	"io"
	"os"
)

type jsonConfig struct {
	data map[string]any
}

func NewJsonConfig(filePath string) Config {
	b, err := os.ReadFile(filePath)
	if err != nil {
		panic(err)
	}
	return NewJsonConfigFromString(string(b))
}

func NewJsonConfigFromString(content string) Config {
	var data map[string]any
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		panic(err)
	}
	return &jsonConfig{data: data}
}

func NewJsonConfigFromReader(r io.Reader) Config {
	var data map[string]any
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		panic(err)
	}
	return &jsonConfig{data: data}
}

func (c *jsonConfig) ReadInt(key string) int64 {
	return toInt64(c.data[key])
}

func (c *jsonConfig) ReadString(key string) string {
	return toString(c.data[key])
}

func (c *jsonConfig) ReadFloat(key string) float64 {
	return toFloat64(c.data[key])
}
