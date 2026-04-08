package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HTTP_HOST", "")
	t.Setenv("PORT", "")
	t.Setenv("BASIC_AUTH_USER", "")
	t.Setenv("BASIC_AUTH_PASSWORD", "")
	t.Setenv("CONTROLLER_DATA_DIR", "")

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
}

func TestLoadWithPortAndBasicAuth(t *testing.T) {
	t.Setenv("HTTP_HOST", "0.0.0.0")
	t.Setenv("PORT", "9090")
	t.Setenv("BASIC_AUTH_USER", "admin")
	t.Setenv("BASIC_AUTH_PASSWORD", "secret")
	t.Setenv("CONTROLLER_DATA_DIR", "/var/lib/neon/controller")

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
	t.Setenv("DOCKER_SOCKET_PATH", "")
	t.Setenv("DOCKER_COMPOSE_PROJECT", "")

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

	if cfg.DockerSocketPath != defaultDockerSocketPath {
		t.Fatalf("expected docker socket path %q, got %q", defaultDockerSocketPath, cfg.DockerSocketPath)
	}

	if cfg.DockerComposeProject != defaultDockerComposeProject {
		t.Fatalf("expected docker compose project %q, got %q", defaultDockerComposeProject, cfg.DockerComposeProject)
	}
}

func TestLoadPrimaryEndpointDockerSettings(t *testing.T) {
	t.Setenv("PRIMARY_ENDPOINT_MODE", "docker")
	t.Setenv("PRIMARY_ENDPOINT_SERVICE", "compute-main")
	t.Setenv("PRIMARY_ENDPOINT_HOST", "10.0.0.1")
	t.Setenv("PRIMARY_ENDPOINT_PORT", "15432")
	t.Setenv("PRIMARY_ENDPOINT_DATABASE", "app")
	t.Setenv("PRIMARY_ENDPOINT_USER", "app_user")
	t.Setenv("DOCKER_SOCKET_PATH", "/custom/docker.sock")
	t.Setenv("DOCKER_COMPOSE_PROJECT", "custom-project")

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

	if cfg.DockerSocketPath != "/custom/docker.sock" {
		t.Fatalf("expected docker socket path %q, got %q", "/custom/docker.sock", cfg.DockerSocketPath)
	}

	if cfg.DockerComposeProject != "custom-project" {
		t.Fatalf("expected docker compose project %q, got %q", "custom-project", cfg.DockerComposeProject)
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
