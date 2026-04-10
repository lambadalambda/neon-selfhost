package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultHTTPPort = 8080
const defaultHTTPHost = "127.0.0.1"
const allowInsecureHTTPBindEnv = "ALLOW_INSECURE_HTTP_BIND"

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
	defaultBranchEndpointBindHost  = "127.0.0.1"
	defaultBranchEndpointPortStart = 56000
	defaultBranchEndpointPortEnd   = 56049
	defaultBranchEndpointIdleStop  = 10 * time.Minute
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
	PrimaryEndpointPassword string

	DockerSocketPath     string
	DockerComposeProject string

	PageserverAPI       string
	PageserverPGVersion int

	BranchEndpointBindHost  string
	BranchEndpointPortStart int
	BranchEndpointPortEnd   int
	BranchEndpointIdleStop  time.Duration
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
	allowInsecureHTTPBind, err := parseOptionalBoolEnv(allowInsecureHTTPBindEnv)
	if err != nil {
		return Config{}, err
	}
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

	primaryEndpointPassword, exists := os.LookupEnv("PRIMARY_ENDPOINT_PASSWORD")
	if !exists || primaryEndpointPassword == "" {
		primaryEndpointPassword = primaryEndpointUser
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

	branchEndpointBindHost := strings.TrimSpace(os.Getenv("BRANCH_ENDPOINT_BIND_HOST"))
	if branchEndpointBindHost == "" {
		branchEndpointBindHost = defaultBranchEndpointBindHost
	}

	branchEndpointPortStart := defaultBranchEndpointPortStart
	if rawPortStart, exists := os.LookupEnv("BRANCH_ENDPOINT_PORT_START"); exists && rawPortStart != "" {
		parsedPortStart, err := strconv.Atoi(rawPortStart)
		if err != nil || parsedPortStart < 1 || parsedPortStart > 65535 {
			return Config{}, fmt.Errorf("invalid BRANCH_ENDPOINT_PORT_START %q", rawPortStart)
		}
		branchEndpointPortStart = parsedPortStart
	}

	branchEndpointPortEnd := defaultBranchEndpointPortEnd
	if rawPortEnd, exists := os.LookupEnv("BRANCH_ENDPOINT_PORT_END"); exists && rawPortEnd != "" {
		parsedPortEnd, err := strconv.Atoi(rawPortEnd)
		if err != nil || parsedPortEnd < 1 || parsedPortEnd > 65535 {
			return Config{}, fmt.Errorf("invalid BRANCH_ENDPOINT_PORT_END %q", rawPortEnd)
		}
		branchEndpointPortEnd = parsedPortEnd
	}

	if branchEndpointPortEnd < branchEndpointPortStart {
		return Config{}, fmt.Errorf("BRANCH_ENDPOINT_PORT_END must be greater than or equal to BRANCH_ENDPOINT_PORT_START")
	}

	branchEndpointIdleStop := defaultBranchEndpointIdleStop
	if rawIdleStop, exists := os.LookupEnv("BRANCH_ENDPOINT_IDLE_TIMEOUT"); exists && strings.TrimSpace(rawIdleStop) != "" {
		parsedIdleStop, err := time.ParseDuration(strings.TrimSpace(rawIdleStop))
		if err != nil || parsedIdleStop <= 0 {
			return Config{}, fmt.Errorf("invalid BRANCH_ENDPOINT_IDLE_TIMEOUT %q", rawIdleStop)
		}

		branchEndpointIdleStop = parsedIdleStop
	}

	if basicAuthUser != "" && basicAuthPassword == "" {
		return Config{}, fmt.Errorf("BASIC_AUTH_PASSWORD is required when BASIC_AUTH_USER is set")
	}

	if basicAuthUser == "" && basicAuthPassword != "" {
		return Config{}, fmt.Errorf("BASIC_AUTH_USER is required when BASIC_AUTH_PASSWORD is set")
	}

	if !allowInsecureHTTPBind && !isLoopbackHTTPHost(host) && basicAuthUser == "" {
		return Config{}, fmt.Errorf("BASIC_AUTH_USER and BASIC_AUTH_PASSWORD are required when HTTP_HOST %q is non-loopback (set %s=1 to override for local testing)", host, allowInsecureHTTPBindEnv)
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
		PrimaryEndpointPassword: primaryEndpointPassword,

		DockerSocketPath:     dockerSocketPath,
		DockerComposeProject: dockerComposeProject,

		PageserverAPI:       pageserverAPI,
		PageserverPGVersion: pageserverPGVersion,

		BranchEndpointBindHost:  branchEndpointBindHost,
		BranchEndpointPortStart: branchEndpointPortStart,
		BranchEndpointPortEnd:   branchEndpointPortEnd,
		BranchEndpointIdleStop:  branchEndpointIdleStop,
	}, nil
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.HTTPHost, c.HTTPPort)
}

func parseOptionalBoolEnv(name string) (bool, error) {
	raw, exists := os.LookupEnv(name)
	if !exists {
		return false, nil
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, nil
	}

	parsed, err := strconv.ParseBool(trimmed)
	if err != nil {
		return false, fmt.Errorf("invalid %s %q", name, raw)
	}

	return parsed, nil
}

func isLoopbackHTTPHost(host string) bool {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return true
	}

	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") && len(trimmed) > 2 {
		trimmed = trimmed[1 : len(trimmed)-1]
	}

	if strings.EqualFold(trimmed, "localhost") {
		return true
	}

	ip := net.ParseIP(trimmed)
	return ip != nil && ip.IsLoopback()
}
