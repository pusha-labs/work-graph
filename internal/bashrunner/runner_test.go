package bashrunner

import (
	"strings"
	"testing"
	"time"
)

func TestExecuteCapturesOutputAndExitCode(t *testing.T) {
	status, result, message := execute(t.Context(), "printf hello; printf problem >&2", 2)
	if status != "succeeded" || message != "" || result["stdout"] != "hello" || result["stderr"] != "problem" || result["exitCode"] != 0 {
		t.Fatalf("unexpected result: %s %#v %q", status, result, message)
	}
}

func TestExecuteReportsFailure(t *testing.T) {
	status, result, _ := execute(t.Context(), "exit 7", 2)
	if status != "failed" || result["exitCode"] != 7 {
		t.Fatalf("unexpected result: %s %#v", status, result)
	}
}

func TestExecuteTimesOut(t *testing.T) {
	started := time.Now()
	status, _, _ := execute(t.Context(), "sleep 5", 1)
	if status != "timed_out" || time.Since(started) > 3*time.Second {
		t.Fatalf("timeout was not enforced: %s", status)
	}
}

func TestOutputIsTruncated(t *testing.T) {
	status, result, _ := execute(t.Context(), "yes x | head -c 300000", 2)
	if status != "succeeded" || !result["outputTruncated"].(bool) || len(result["stdout"].(string)) != maxOutputBytes || !strings.HasPrefix(result["stdout"].(string), "x") {
		t.Fatalf("output limit was not enforced")
	}
}
