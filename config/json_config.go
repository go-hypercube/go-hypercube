package config

import (
	"encoding/json"
	"io"
	"os"
)

type jsonConfig struct {
	data map[string]any
}

func NewJsonConfig(filePath string) (Config, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return NewJsonConfigFromString(string(b))
}

func NewJsonConfigFromString(content string) (Config, error) {
	var data map[string]any
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		return nil, err
	}
	return &jsonConfig{data: data}, nil
}

func NewJsonConfigFromReader(r io.Reader) (Config, error) {
	var data map[string]any
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return nil, err
	}
	return &jsonConfig{data: data}, nil
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
