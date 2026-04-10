package config

import (
	"os"
)

type Config struct {
	ListenAddr string
	AuthUser   string
	AuthPass   string
}

func Load() *Config {
	return &Config{
		ListenAddr: getEnv("LISTEN_ADDR", ":8080"),
		AuthUser:   getEnv("AUTH_USER", "admin"),
		AuthPass:   getEnv("AUTH_PASS", "password"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
