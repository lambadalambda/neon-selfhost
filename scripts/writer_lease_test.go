package scripts

import (
	"bufio"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriterLeaseSerializesAttachmentOwners(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("writer lease behavior requires Linux flock semantics")
	}
	if _, err := exec.LookPath("flock"); err != nil {
		t.Fatalf("flock is required on Linux: %v", err)
	}

	script, err := filepath.Abs("../configs/neon/compute_wrapper/shell/writer_lease.sh")
	if err != nil {
		t.Fatalf("resolve writer lease script: %v", err)
	}
	leaseDir := t.TempDir()
	tenantID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	timelineID := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	owner := exec.Command("bash", "-c", `source "$1"; acquire_writer_lease "$2" "$3" "$4"; printf 'ready\n'; exec sleep 30`, "writer-lease-owner", script, tenantID, timelineID, leaseDir)
	stdout, err := owner.StdoutPipe()
	if err != nil {
		t.Fatalf("owner stdout: %v", err)
	}
	if err := owner.Start(); err != nil {
		t.Fatalf("start lease owner: %v", err)
	}
	t.Cleanup(func() {
		_ = owner.Process.Kill()
		_ = owner.Wait()
	})
	if scanner := bufio.NewScanner(stdout); !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatalf("writer lease owner did not become ready")
	}

	assertWriterLeaseExitCode(t, script, tenantID, timelineID, leaseDir, 75)
	assertWriterLeaseExitCode(t, script, tenantID, "cccccccccccccccccccccccccccccccc", leaseDir, 0)
	assertWriterLeaseExitCode(t, script, "../invalid", timelineID, leaseDir, 64)

	if err := owner.Process.Kill(); err != nil {
		t.Fatalf("stop lease owner: %v", err)
	}
	if err := owner.Wait(); err == nil {
		t.Fatal("expected killed owner to return an error")
	}
	assertWriterLeaseExitCode(t, script, tenantID, timelineID, leaseDir, 0)
}

func assertWriterLeaseExitCode(t *testing.T, script string, tenantID string, timelineID string, leaseDir string, want int) {
	t.Helper()
	cmd := exec.Command("bash", "-c", `source "$1"; acquire_writer_lease "$2" "$3" "$4"`, "writer-lease-contender", script, tenantID, timelineID, leaseDir)
	err := cmd.Run()
	if want == 0 {
		if err != nil {
			t.Fatalf("expected writer lease success, got %v", err)
		}
		return
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != want {
		t.Fatalf("expected writer lease exit %d, got %v", want, err)
	}
}
