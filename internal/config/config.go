package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultHTTPPort = 8080
const defaultHTTPHost = "127.0.0.1"

const (
	defaultPrimaryEndpointMode     = "memory"
	primaryEndpointModeMemory      = "memory"
	primaryEndpointModeDocker      = "docker"
	defaultPrimaryEndpointService  = "compute"
	defaultPrimaryEndpointHost     = "127.0.0.1"
	defaultPrimaryEndpointPort     = 5432
	defaultPrimaryEndpointDatabase = "postgres"
	defaultPrimaryEndpointUser     = "cloud_admin"
	defaultDockerSocketPath        = "/var/run/docker.sock"
	defaultDockerComposeProject    = "neon-selfhost"
	defaultPageserverAPI           = "http://pageserver:9898"
	defaultPageserverPGVersion     = 16
)

type Config struct {
	HTTPHost          string
	HTTPPort          int
	BasicAuthUser     string
	BasicAuthPassword string
	ControllerDataDir string
	ComputeDataDir    string

	PrimaryEndpointMode     string
	PrimaryEndpointService  string
	PrimaryEndpointHost     string
	PrimaryEndpointPort     int
	PrimaryEndpointDatabase string
	PrimaryEndpointUser     string

	DockerSocketPath     string
	DockerComposeProject string

	PageserverAPI       string
	PageserverPGVersion int
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
	computeDataDir := strings.TrimSpace(os.Getenv("COMPUTE_DATA_DIR"))

	primaryEndpointMode := strings.ToLower(strings.TrimSpace(os.Getenv("PRIMARY_ENDPOINT_MODE")))
	if primaryEndpointMode == "" {
		primaryEndpointMode = defaultPrimaryEndpointMode
	}

	switch primaryEndpointMode {
	case primaryEndpointModeMemory, primaryEndpointModeDocker:
	default:
		return Config{}, fmt.Errorf("invalid PRIMARY_ENDPOINT_MODE %q", primaryEndpointMode)
	}

	primaryEndpointService := strings.TrimSpace(os.Getenv("PRIMARY_ENDPOINT_SERVICE"))
	if primaryEndpointService == "" {
		primaryEndpointService = defaultPrimaryEndpointService
	}

	primaryEndpointHost := strings.TrimSpace(os.Getenv("PRIMARY_ENDPOINT_HOST"))
	if primaryEndpointHost == "" {
		primaryEndpointHost = defaultPrimaryEndpointHost
	}

	primaryEndpointPort := defaultPrimaryEndpointPort
	if rawPrimaryEndpointPort, exists := os.LookupEnv("PRIMARY_ENDPOINT_PORT"); exists && rawPrimaryEndpointPort != "" {
		parsedPrimaryEndpointPort, err := strconv.Atoi(rawPrimaryEndpointPort)
		if err != nil || parsedPrimaryEndpointPort < 1 || parsedPrimaryEndpointPort > 65535 {
			return Config{}, fmt.Errorf("invalid PRIMARY_ENDPOINT_PORT %q", rawPrimaryEndpointPort)
		}

		primaryEndpointPort = parsedPrimaryEndpointPort
	}

	primaryEndpointDatabase := strings.TrimSpace(os.Getenv("PRIMARY_ENDPOINT_DATABASE"))
	if primaryEndpointDatabase == "" {
		primaryEndpointDatabase = defaultPrimaryEndpointDatabase
	}

	primaryEndpointUser := strings.TrimSpace(os.Getenv("PRIMARY_ENDPOINT_USER"))
	if primaryEndpointUser == "" {
		primaryEndpointUser = defaultPrimaryEndpointUser
	}

	dockerSocketPath := strings.TrimSpace(os.Getenv("DOCKER_SOCKET_PATH"))
	if dockerSocketPath == "" {
		dockerSocketPath = defaultDockerSocketPath
	}

	dockerComposeProject := strings.TrimSpace(os.Getenv("DOCKER_COMPOSE_PROJECT"))
	if dockerComposeProject == "" {
		dockerComposeProject = defaultDockerComposeProject
	}

	pageserverAPI := strings.TrimSpace(os.Getenv("PAGESERVER_API"))
	if pageserverAPI == "" {
		pageserverAPI = defaultPageserverAPI
	}

	pageserverPGVersion := defaultPageserverPGVersion
	if rawPageserverPGVersion, exists := os.LookupEnv("PAGESERVER_PG_VERSION"); exists && rawPageserverPGVersion != "" {
		parsedPageserverPGVersion, err := strconv.Atoi(rawPageserverPGVersion)
		if err != nil || parsedPageserverPGVersion < 1 {
			return Config{}, fmt.Errorf("invalid PAGESERVER_PG_VERSION %q", rawPageserverPGVersion)
		}

		pageserverPGVersion = parsedPageserverPGVersion
	}

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
		ComputeDataDir:    computeDataDir,

		PrimaryEndpointMode:     primaryEndpointMode,
		PrimaryEndpointService:  primaryEndpointService,
		PrimaryEndpointHost:     primaryEndpointHost,
		PrimaryEndpointPort:     primaryEndpointPort,
		PrimaryEndpointDatabase: primaryEndpointDatabase,
		PrimaryEndpointUser:     primaryEndpointUser,

		DockerSocketPath:     dockerSocketPath,
		DockerComposeProject: dockerComposeProject,

		PageserverAPI:       pageserverAPI,
		PageserverPGVersion: pageserverPGVersion,
	}, nil
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.HTTPHost, c.HTTPPort)
}
