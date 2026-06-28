package present

import (
	"testing"

	"github.com/vxider/agent-buddy/internal/model"
)

func TestErrorSummaryPrefersCommandOverRawOutput(t *testing.T) {
	session := model.SessionSnapshot{
		LastBashCommand: "go test ./webserver/...",
		LastError:       "FAIL\tgithub.com/vxider/agent-buddy/webserver/internal/api\t0.007s",
	}

	got := ErrorSummary(session)
	want := "Command failed: go test ./webserver/..."
	if got != want {
		t.Fatalf("unexpected error summary: %q", got)
	}
}

func TestErrorSummaryFallsBackToReadableErrorLine(t *testing.T) {
	session := model.SessionSnapshot{
		LastError: "line 1\nsed: can't read src/api/dataAnnotation.ts: No such file or directory\nstack trace",
	}

	got := ErrorSummary(session)
	want := "sed: can't read src/api/dataAnnotation.ts: No such file or directory"
	if got != want {
		t.Fatalf("unexpected readable error summary: %q", got)
	}
}

func TestErrorTitleUsesCommandFailedWhenCommandExists(t *testing.T) {
	session := model.SessionSnapshot{LastBashCommand: "npm test"}
	if got := ErrorTitle(session); got != "Command failed" {
		t.Fatalf("unexpected error title: %q", got)
	}
}
