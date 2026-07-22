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

func TestComputeRequiresAttachmentWriterLease(t *testing.T) {
	composeContent, err := os.ReadFile("../docker-compose.yml")
	if err != nil {
		t.Fatalf("read compose config: %v", err)
	}
	if !strings.Contains(string(composeContent), `BRANCH_ENDPOINT_COMPUTE_IMAGE: ${NEON_COMPUTE_WRAPPER_IMAGE:-neon-selfhost/compute:dev}`) {
		t.Fatal("expected primary and branch computes to use the configured wrapper image")
	}
	computeDockerfile, err := os.ReadFile("../configs/neon/compute_wrapper/Dockerfile")
	if err != nil {
		t.Fatalf("read compute Dockerfile: %v", err)
	}
	if !strings.Contains(string(computeDockerfile), "util-linux") {
		t.Fatal("expected compute image to install flock explicitly through util-linux")
	}
	compose := string(composeContent)
	for _, expected := range []string{
		"compute_state_init:",
		"condition: service_completed_successfully",
		"chown 65532:65532 /var/lib/neon/compute/writer-leases",
		"chmod 01777 /var/lib/neon/compute/writer-leases",
	} {
		if !strings.Contains(compose, expected) {
			t.Fatalf("expected compute-state initialization to contain %q", expected)
		}
	}

	computeScript, err := os.ReadFile("../configs/neon/compute_wrapper/shell/compute.sh")
	if err != nil {
		t.Fatalf("read compute script: %v", err)
	}
	leaseScript, err := os.ReadFile("../configs/neon/compute_wrapper/shell/writer_lease.sh")
	if err != nil {
		t.Fatalf("read writer lease script: %v", err)
	}

	compute := string(computeScript)
	for _, expected := range []string{
		`source /shell/writer_lease.sh`,
		`acquire_writer_lease "${tenant_id}" "${timeline_id}"`,
	} {
		if !strings.Contains(compute, expected) {
			t.Fatalf("expected compute startup to contain %q", expected)
		}
	}

	lease := string(leaseScript)
	for _, expected := range []string{
		`^[0-9a-fA-F]{32}$`,
		`/var/lib/neon/compute/writer-leases`,
		`exec {WRITER_LEASE_FD}<>`,
		`flock -E 75 -n "${WRITER_LEASE_FD}"`,
	} {
		if !strings.Contains(lease, expected) {
			t.Fatalf("expected writer lease implementation to contain %q", expected)
		}
	}
}

func TestPrimaryComputePublishesAppliedSelectionAndHealth(t *testing.T) {
	composeContent, err := os.ReadFile("../docker-compose.yml")
	if err != nil {
		t.Fatalf("read compose config: %v", err)
	}
	compose := string(composeContent)
	for _, expected := range []string{
		`PRIMARY_ENDPOINT_BACKEND_HOST: ${PRIMARY_ENDPOINT_BACKEND_HOST:-compute}`,
		`PRIMARY_ENDPOINT_BACKEND_PORT: ${PRIMARY_ENDPOINT_BACKEND_PORT:-55433}`,
		`test: ["CMD-SHELL", "pg_isready -h 127.0.0.1 -p 55433 -U cloud_admin -d postgres"]`,
		`chmod 01777 /var/lib/neon/compute/applied`,
	} {
		if !strings.Contains(compose, expected) {
			t.Fatalf("expected primary route Compose config to contain %q", expected)
		}
	}

	computeScript, err := os.ReadFile("../configs/neon/compute_wrapper/shell/compute.sh")
	if err != nil {
		t.Fatalf("read compute script: %v", err)
	}
	compute := string(computeScript)
	for _, expected := range []string{
		`ENDPOINT_APPLIED_FILE=`,
		`rm -f "${ENDPOINT_APPLIED_FILE}"`,
		`mv "${applied_selection_tmp}" "${ENDPOINT_APPLIED_FILE}"`,
	} {
		if !strings.Contains(compute, expected) {
			t.Fatalf("expected compute applied-state marker to contain %q", expected)
		}
	}
}
