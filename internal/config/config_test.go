package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HTTP_HOST", "")
	t.Setenv("PORT", "")
	t.Setenv("BASIC_AUTH_USER", "")
	t.Setenv("BASIC_AUTH_PASSWORD", "")
	t.Setenv("CONTROLLER_DATA_DIR", "")
	t.Setenv("COMPUTE_DATA_DIR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPHost != defaultHTTPHost {
		t.Fatalf("expected default host %q, got %q", defaultHTTPHost, cfg.HTTPHost)
	}

	if cfg.HTTPPort != defaultHTTPPort {
		t.Fatalf("expected default port %d, got %d", defaultHTTPPort, cfg.HTTPPort)
	}

	if cfg.BasicAuthUser != "" {
		t.Fatalf("expected empty basic auth user, got %q", cfg.BasicAuthUser)
	}

	if cfg.BasicAuthPassword != "" {
		t.Fatal("expected empty basic auth password")
	}

	if cfg.ControllerDataDir != "" {
		t.Fatalf("expected empty controller data dir, got %q", cfg.ControllerDataDir)
	}

	if cfg.ComputeDataDir != "" {
		t.Fatalf("expected empty compute data dir, got %q", cfg.ComputeDataDir)
	}

}

func TestLoadWithPortAndBasicAuth(t *testing.T) {
	t.Setenv("HTTP_HOST", "0.0.0.0")
	t.Setenv("PORT", "9090")
	t.Setenv("BASIC_AUTH_USER", "admin")
	t.Setenv("BASIC_AUTH_PASSWORD", "secret")
	t.Setenv("CONTROLLER_DATA_DIR", "/var/lib/neon/controller")
	t.Setenv("COMPUTE_DATA_DIR", "/var/lib/neon/compute")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPHost != "0.0.0.0" {
		t.Fatalf("expected host %q, got %q", "0.0.0.0", cfg.HTTPHost)
	}

	if cfg.HTTPPort != 9090 {
		t.Fatalf("expected port %d, got %d", 9090, cfg.HTTPPort)
	}

	if cfg.BasicAuthUser != "admin" {
		t.Fatalf("expected basic auth user %q, got %q", "admin", cfg.BasicAuthUser)
	}

	if cfg.BasicAuthPassword != "secret" {
		t.Fatal("expected basic auth password to be loaded")
	}

	if cfg.ControllerDataDir != "/var/lib/neon/controller" {
		t.Fatalf("expected controller data dir %q, got %q", "/var/lib/neon/controller", cfg.ControllerDataDir)
	}

	if cfg.ComputeDataDir != "/var/lib/neon/compute" {
		t.Fatalf("expected compute data dir %q, got %q", "/var/lib/neon/compute", cfg.ComputeDataDir)
	}

}

func TestLoadRejectsMissingBasicAuthPassword(t *testing.T) {
	t.Setenv("BASIC_AUTH_USER", "admin")
	t.Setenv("BASIC_AUTH_PASSWORD", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing basic auth password")
	}
}

func TestLoadRejectsMissingBasicAuthUser(t *testing.T) {
	t.Setenv("BASIC_AUTH_USER", "")
	t.Setenv("BASIC_AUTH_PASSWORD", "secret")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing basic auth user")
	}
}

func TestLoadPrimaryEndpointDefaults(t *testing.T) {
	t.Setenv("PRIMARY_ENDPOINT_MODE", "")
	t.Setenv("PRIMARY_ENDPOINT_SERVICE", "")
	t.Setenv("PRIMARY_ENDPOINT_HOST", "")
	t.Setenv("PRIMARY_ENDPOINT_PORT", "")
	t.Setenv("PRIMARY_ENDPOINT_DATABASE", "")
	t.Setenv("PRIMARY_ENDPOINT_USER", "")
	t.Setenv("PRIMARY_ENDPOINT_PASSWORD", "")
	t.Setenv("DOCKER_SOCKET_PATH", "")
	t.Setenv("DOCKER_COMPOSE_PROJECT", "")
	t.Setenv("PAGESERVER_API", "")
	t.Setenv("PAGESERVER_PG_VERSION", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.PrimaryEndpointMode != defaultPrimaryEndpointMode {
		t.Fatalf("expected primary endpoint mode %q, got %q", defaultPrimaryEndpointMode, cfg.PrimaryEndpointMode)
	}

	if cfg.PrimaryEndpointService != defaultPrimaryEndpointService {
		t.Fatalf("expected primary endpoint service %q, got %q", defaultPrimaryEndpointService, cfg.PrimaryEndpointService)
	}

	if cfg.PrimaryEndpointHost != defaultPrimaryEndpointHost {
		t.Fatalf("expected primary endpoint host %q, got %q", defaultPrimaryEndpointHost, cfg.PrimaryEndpointHost)
	}

	if cfg.PrimaryEndpointPort != defaultPrimaryEndpointPort {
		t.Fatalf("expected primary endpoint port %d, got %d", defaultPrimaryEndpointPort, cfg.PrimaryEndpointPort)
	}

	if cfg.PrimaryEndpointDatabase != defaultPrimaryEndpointDatabase {
		t.Fatalf("expected primary endpoint database %q, got %q", defaultPrimaryEndpointDatabase, cfg.PrimaryEndpointDatabase)
	}

	if cfg.PrimaryEndpointUser != defaultPrimaryEndpointUser {
		t.Fatalf("expected primary endpoint user %q, got %q", defaultPrimaryEndpointUser, cfg.PrimaryEndpointUser)
	}

	if cfg.PrimaryEndpointPassword != defaultPrimaryEndpointUser {
		t.Fatalf("expected primary endpoint password %q, got %q", defaultPrimaryEndpointUser, cfg.PrimaryEndpointPassword)
	}

	if cfg.DockerSocketPath != defaultDockerSocketPath {
		t.Fatalf("expected docker socket path %q, got %q", defaultDockerSocketPath, cfg.DockerSocketPath)
	}

	if cfg.DockerComposeProject != defaultDockerComposeProject {
		t.Fatalf("expected docker compose project %q, got %q", defaultDockerComposeProject, cfg.DockerComposeProject)
	}

	if cfg.PageserverAPI != defaultPageserverAPI {
		t.Fatalf("expected pageserver api %q, got %q", defaultPageserverAPI, cfg.PageserverAPI)
	}

	if cfg.PageserverPGVersion != defaultPageserverPGVersion {
		t.Fatalf("expected pageserver pg version %d, got %d", defaultPageserverPGVersion, cfg.PageserverPGVersion)
	}

	if cfg.BranchEndpointBindHost != defaultBranchEndpointBindHost {
		t.Fatalf("expected branch endpoint bind host %q, got %q", defaultBranchEndpointBindHost, cfg.BranchEndpointBindHost)
	}

	if cfg.BranchEndpointPortStart != defaultBranchEndpointPortStart {
		t.Fatalf("expected branch endpoint port start %d, got %d", defaultBranchEndpointPortStart, cfg.BranchEndpointPortStart)
	}

	if cfg.BranchEndpointPortEnd != defaultBranchEndpointPortEnd {
		t.Fatalf("expected branch endpoint port end %d, got %d", defaultBranchEndpointPortEnd, cfg.BranchEndpointPortEnd)
	}

	if cfg.BranchEndpointIdleStop != defaultBranchEndpointIdleStop {
		t.Fatalf("expected branch endpoint idle stop %s, got %s", defaultBranchEndpointIdleStop, cfg.BranchEndpointIdleStop)
	}

	if cfg.BranchEndpointMaxConns != defaultBranchEndpointMaxConns {
		t.Fatalf("expected branch endpoint max conns %d, got %d", defaultBranchEndpointMaxConns, cfg.BranchEndpointMaxConns)
	}
}

func TestLoadPrimaryEndpointDockerSettings(t *testing.T) {
	t.Setenv("PRIMARY_ENDPOINT_MODE", "docker")
	t.Setenv("PRIMARY_ENDPOINT_SERVICE", "compute-main")
	t.Setenv("PRIMARY_ENDPOINT_HOST", "10.0.0.1")
	t.Setenv("PRIMARY_ENDPOINT_PORT", "15432")
	t.Setenv("PRIMARY_ENDPOINT_DATABASE", "app")
	t.Setenv("PRIMARY_ENDPOINT_USER", "app_user")
	t.Setenv("PRIMARY_ENDPOINT_PASSWORD", "app_secret")
	t.Setenv("DOCKER_SOCKET_PATH", "/custom/docker.sock")
	t.Setenv("DOCKER_COMPOSE_PROJECT", "custom-project")
	t.Setenv("PAGESERVER_API", "http://pageserver.internal:9898")
	t.Setenv("PAGESERVER_PG_VERSION", "17")
	t.Setenv("BRANCH_ENDPOINT_BIND_HOST", "0.0.0.0")
	t.Setenv("BRANCH_ENDPOINT_PORT_START", "56100")
	t.Setenv("BRANCH_ENDPOINT_PORT_END", "56199")
	t.Setenv("BRANCH_ENDPOINT_IDLE_TIMEOUT", "45s")
	t.Setenv("BRANCH_ENDPOINT_MAX_CONNECTIONS", "48")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.PrimaryEndpointMode != "docker" {
		t.Fatalf("expected primary endpoint mode %q, got %q", "docker", cfg.PrimaryEndpointMode)
	}

	if cfg.PrimaryEndpointService != "compute-main" {
		t.Fatalf("expected primary endpoint service %q, got %q", "compute-main", cfg.PrimaryEndpointService)
	}

	if cfg.PrimaryEndpointHost != "10.0.0.1" {
		t.Fatalf("expected primary endpoint host %q, got %q", "10.0.0.1", cfg.PrimaryEndpointHost)
	}

	if cfg.PrimaryEndpointPort != 15432 {
		t.Fatalf("expected primary endpoint port %d, got %d", 15432, cfg.PrimaryEndpointPort)
	}

	if cfg.PrimaryEndpointDatabase != "app" {
		t.Fatalf("expected primary endpoint database %q, got %q", "app", cfg.PrimaryEndpointDatabase)
	}

	if cfg.PrimaryEndpointUser != "app_user" {
		t.Fatalf("expected primary endpoint user %q, got %q", "app_user", cfg.PrimaryEndpointUser)
	}

	if cfg.PrimaryEndpointPassword != "app_secret" {
		t.Fatalf("expected primary endpoint password %q, got %q", "app_secret", cfg.PrimaryEndpointPassword)
	}

	if cfg.DockerSocketPath != "/custom/docker.sock" {
		t.Fatalf("expected docker socket path %q, got %q", "/custom/docker.sock", cfg.DockerSocketPath)
	}

	if cfg.DockerComposeProject != "custom-project" {
		t.Fatalf("expected docker compose project %q, got %q", "custom-project", cfg.DockerComposeProject)
	}

	if cfg.PageserverAPI != "http://pageserver.internal:9898" {
		t.Fatalf("expected pageserver api %q, got %q", "http://pageserver.internal:9898", cfg.PageserverAPI)
	}

	if cfg.PageserverPGVersion != 17 {
		t.Fatalf("expected pageserver pg version %d, got %d", 17, cfg.PageserverPGVersion)
	}

	if cfg.BranchEndpointBindHost != "0.0.0.0" {
		t.Fatalf("expected branch endpoint bind host %q, got %q", "0.0.0.0", cfg.BranchEndpointBindHost)
	}

	if cfg.BranchEndpointPortStart != 56100 {
		t.Fatalf("expected branch endpoint port start %d, got %d", 56100, cfg.BranchEndpointPortStart)
	}

	if cfg.BranchEndpointPortEnd != 56199 {
		t.Fatalf("expected branch endpoint port end %d, got %d", 56199, cfg.BranchEndpointPortEnd)
	}

	if cfg.BranchEndpointIdleStop != 45*time.Second {
		t.Fatalf("expected branch endpoint idle stop %s, got %s", 45*time.Second, cfg.BranchEndpointIdleStop)
	}

	if cfg.BranchEndpointMaxConns != 48 {
		t.Fatalf("expected branch endpoint max conns %d, got %d", 48, cfg.BranchEndpointMaxConns)
	}
}

func TestLoadRejectsInvalidPrimaryEndpointMode(t *testing.T) {
	t.Setenv("PRIMARY_ENDPOINT_MODE", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid primary endpoint mode")
	}
}

func TestLoadRejectsInvalidPrimaryEndpointPort(t *testing.T) {
	t.Setenv("PRIMARY_ENDPOINT_PORT", "99999")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid primary endpoint port")
	}
}

func TestLoadRejectsInvalidPageserverPGVersion(t *testing.T) {
	t.Setenv("PAGESERVER_PG_VERSION", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid pageserver pg version")
	}
}

func TestLoadRejectsInvalidBranchEndpointPortRange(t *testing.T) {
	t.Setenv("BRANCH_ENDPOINT_PORT_START", "56200")
	t.Setenv("BRANCH_ENDPOINT_PORT_END", "56100")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid branch endpoint port range")
	}
}

func TestLoadRejectsUnauthenticatedNonLoopbackBind(t *testing.T) {
	t.Setenv("HTTP_HOST", "0.0.0.0")
	t.Setenv("BASIC_AUTH_USER", "")
	t.Setenv("BASIC_AUTH_PASSWORD", "")
	t.Setenv("ALLOW_INSECURE_HTTP_BIND", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for non-loopback bind without basic auth")
	}
}

func TestLoadAllowsUnauthenticatedNonLoopbackBindWithOverride(t *testing.T) {
	t.Setenv("HTTP_HOST", "0.0.0.0")
	t.Setenv("BASIC_AUTH_USER", "")
	t.Setenv("BASIC_AUTH_PASSWORD", "")
	t.Setenv("ALLOW_INSECURE_HTTP_BIND", "1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPHost != "0.0.0.0" {
		t.Fatalf("expected host %q, got %q", "0.0.0.0", cfg.HTTPHost)
	}
}

func TestLoadRejectsInvalidAllowInsecureHTTPBindValue(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_HTTP_BIND", "maybe")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid ALLOW_INSECURE_HTTP_BIND")
	}
}

func TestLoadRejectsInvalidBranchEndpointIdleTimeout(t *testing.T) {
	t.Setenv("BRANCH_ENDPOINT_IDLE_TIMEOUT", "0s")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid BRANCH_ENDPOINT_IDLE_TIMEOUT")
	}
}

func TestLoadRejectsInvalidBranchEndpointMaxConnections(t *testing.T) {
	t.Setenv("BRANCH_ENDPOINT_MAX_CONNECTIONS", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid BRANCH_ENDPOINT_MAX_CONNECTIONS")
	}
}
