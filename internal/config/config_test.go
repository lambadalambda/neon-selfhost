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
