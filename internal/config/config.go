package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultHTTPPort = 8080
const defaultHTTPHost = "127.0.0.1"

type Config struct {
	HTTPHost          string
	HTTPPort          int
	BasicAuthUser     string
	BasicAuthPassword string
	ControllerDataDir string
}

func Load() (Config, error) {
	host := os.Getenv("HTTP_HOST")
	if host == "" {
		host = defaultHTTPHost
	}

	port := defaultHTTPPort
	rawPort, exists := os.LookupEnv("PORT")
	if exists && rawPort != "" {
		parsedPort, err := strconv.Atoi(rawPort)
		if err != nil || parsedPort < 1 || parsedPort > 65535 {
			return Config{}, fmt.Errorf("invalid PORT %q", rawPort)
		}

		port = parsedPort
	}

	basicAuthUser := strings.TrimSpace(os.Getenv("BASIC_AUTH_USER"))
	basicAuthPassword := os.Getenv("BASIC_AUTH_PASSWORD")
	controllerDataDir := strings.TrimSpace(os.Getenv("CONTROLLER_DATA_DIR"))

	if basicAuthUser != "" && basicAuthPassword == "" {
		return Config{}, fmt.Errorf("BASIC_AUTH_PASSWORD is required when BASIC_AUTH_USER is set")
	}

	if basicAuthUser == "" && basicAuthPassword != "" {
		return Config{}, fmt.Errorf("BASIC_AUTH_USER is required when BASIC_AUTH_PASSWORD is set")
	}

	return Config{
		HTTPHost:          host,
		HTTPPort:          port,
		BasicAuthUser:     basicAuthUser,
		BasicAuthPassword: basicAuthPassword,
		ControllerDataDir: controllerDataDir,
	}, nil
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.HTTPHost, c.HTTPPort)
}
