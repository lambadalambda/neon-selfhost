package scripts

import (
	"os"
	"strings"
	"testing"
)

func TestComposeRequiresEndpointPasswordAndRunsControllerNonRoot(t *testing.T) {
	content, err := os.ReadFile("../docker-compose.yml")
	if err != nil {
		t.Fatalf("read compose config: %v", err)
	}
	compose := string(content)
	for _, expected := range []string{
		`PRIMARY_ENDPOINT_PASSWORD: ${PRIMARY_ENDPOINT_PASSWORD:?`,
		`user: "65532:0"`,
		`${CONTAINER_ENGINE_SOCKET:-/run/podman/podman.sock}:/var/run/docker.sock`,
	} {
		if !strings.Contains(compose, expected) {
			t.Fatalf("expected Compose config to contain %q", expected)
		}
	}
	if strings.Contains(compose, `user: "0:0"`) {
		t.Fatal("controller must not run as root")
	}
	if count := strings.Count(compose, `PRIMARY_ENDPOINT_PASSWORD: ${PRIMARY_ENDPOINT_PASSWORD:?`); count != 2 {
		t.Fatalf("expected controller and compute to require PRIMARY_ENDPOINT_PASSWORD, got %d declarations", count)
	}
}

func TestOperationalTasksUsePodmanCompose(t *testing.T) {
	for _, path := range []string{"../mise.toml", "smoke.sh", "reset_seed_data.sh"} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(content), "docker compose") {
			t.Fatalf("expected %s to use Podman Compose", path)
		}
		if !strings.Contains(string(content), "PRIMARY_ENDPOINT_PASSWORD") {
			t.Fatalf("expected %s to propagate PRIMARY_ENDPOINT_PASSWORD", path)
		}
		if path != "../mise.toml" && !strings.Contains(string(content), "require_command podman") {
			t.Fatalf("expected %s to require Podman for managed stack mode", path)
		}
		if strings.Contains(string(content), "require_command docker") {
			t.Fatalf("expected %s not to require Docker", path)
		}
	}
}
