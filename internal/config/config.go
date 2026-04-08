package config

import (
	"fmt"
	"os"
	"strconv"
)

const defaultHTTPPort = 8080
const defaultHTTPHost = "127.0.0.1"

type Config struct {
	HTTPHost string
	HTTPPort int
}

func Load() (Config, error) {
	host := os.Getenv("HTTP_HOST")
	if host == "" {
		host = defaultHTTPHost
	}

	rawPort, exists := os.LookupEnv("PORT")
	if !exists || rawPort == "" {
		return Config{HTTPHost: host, HTTPPort: defaultHTTPPort}, nil
	}

	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("invalid PORT %q", rawPort)
	}

	return Config{HTTPHost: host, HTTPPort: port}, nil
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.HTTPHost, c.HTTPPort)
}
