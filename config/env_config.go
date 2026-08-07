package config

import (
	"io"
	"strconv"

	"github.com/joho/godotenv"
)

type envConfig struct {
	data map[string]string
}

func NewEnvConfig(filePath string) Config {
	data, err := godotenv.Read(filePath)
	if err != nil {
		panic(err)
	}
	return &envConfig{data: data}
}

func NewEnvConfigFromString(content string) Config {
	data, err := godotenv.Unmarshal(content)
	if err != nil {
		panic(err)
	}
	return &envConfig{data: data}
}

func NewEnvConfigFromReader(r io.Reader) Config {
	data, err := godotenv.Parse(r)
	if err != nil {
		panic(err)
	}
	return &envConfig{data: data}
}

func (c *envConfig) ReadInt(key string) int64 {
	i, _ := strconv.ParseInt(c.data[key], 10, 64)
	return i
}

func (c *envConfig) ReadString(key string) string {
	return c.data[key]
}

func (c *envConfig) ReadFloat(key string) float64 {
	f, _ := strconv.ParseFloat(c.data[key], 64)
	return f
}
