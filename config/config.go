package config

type Config interface {
	ReadInt(key string) int64
	ReadString(key string) string
	ReadFloat(key string) float64
	AppKey() string
}
