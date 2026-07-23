package scripts

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSmokeWriterHandoffRequiresManagedStack(t *testing.T) {
	cmd := exec.Command("bash", "smoke.sh", "--verify-writer-handoff")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected writer handoff smoke to reject an unmanaged stack")
	}
	if !strings.Contains(string(output), "--verify-writer-handoff requires --manage-stack") {
		t.Fatalf("unexpected guard output: %s", output)
	}
}

func TestSmokeWriterHandoffRejectsRemoteAPI(t *testing.T) {
	for _, baseURL := range []string{
		"https://fediffusion.art",
		"http://127.0.0.1:8080@fediffusion.art",
		"http://127.0.0.1:9999",
	} {
		t.Run(baseURL, func(t *testing.T) {
			cmd := exec.Command("bash", "smoke.sh", "--manage-stack", "--verify-writer-handoff")
			cmd.Env = append(os.Environ(), "BASE_URL="+baseURL, "CONTROLLER_HOST_PORT=8080")
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("expected writer handoff smoke to reject a remote or mismatched API")
			}
			if !strings.Contains(string(output), "--verify-writer-handoff requires BASE_URL to match the managed loopback port") {
				t.Fatalf("unexpected guard output: %s", output)
			}
		})
	}
}

func TestSmokeWriterHandoffIncludesSafetyAssertions(t *testing.T) {
	content, err := exec.Command("bash", "-n", "smoke.sh").CombinedOutput()
	if err != nil {
		t.Fatalf("smoke script syntax: %v: %s", err, content)
	}

	script, err := os.ReadFile("smoke.sh")
	if err != nil {
		t.Fatalf("read smoke script: %v", err)
	}
	for _, expected := range []string{
		"neon.selfhost.endpoint=branch",
		"neon.selfhost.branch=",
		"lazy target compute was not removed",
		"target SQL did not route through primary after handoff",
		"CREATE TABLE public.${PROBE_TABLE}",
		"UPDATE public.${PROBE_TABLE}",
		"compose down --volumes",
		"retaining disposable stack ${COMPOSE_PROJECT} because primary handback failed",
		`COMPOSE_PROJECT="nsh-$$-${RANDOM}"`,
		`REQUEST_BASE_URL="http://controller:8080"`,
	} {
		if !strings.Contains(string(script), expected) {
			t.Fatalf("expected writer handoff smoke to contain %q", expected)
		}
	}
}

func TestSmokeWriterHandoffNamesFitDNSLabelLimit(t *testing.T) {
	project := "nsh-2147483647-32767"
	branch := "smoke-20260723060518-32767"
	containerName := project + "-branch-" + branch + "-ffffffff"
	if len(containerName) > 63 {
		t.Fatalf("generated branch compute name exceeds DNS label limit: %d: %s", len(containerName), containerName)
	}
}
