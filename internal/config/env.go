// @guardian-project: guardian
// @guardian-path: internal/config/env.go
package config

import (
	"os"
	"path/filepath"
)

const (
	DefaultHTTPAddr      = "127.0.0.1:8085"
	DefaultNexusAddr     = "http://127.0.0.1:8080"
	DefaultForgeAddr     = "http://127.0.0.1:8082"
	DefaultNavigatorAddr = "http://127.0.0.1:8084"
)

func EnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func ExpandHome(path string) string {
	if len(path) < 2 || path[:2] != "~/" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
