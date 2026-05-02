//go:build uconsole_gui

package uconsole

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisplayLocalLinkTargetShortensPathUnderCWD(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}

	target := filepath.Join(cwd, "uconsole", "internal", "app.go") + ":42"
	if got, want := displayLocalLinkTarget(target), "uconsole/internal/app.go:42"; got != want {
		t.Fatalf("displayLocalLinkTarget() = %q, want %q", got, want)
	}
}

func TestDisplayLocalLinkTargetKeepsHashLocationSuffix(t *testing.T) {
	if got, want := displayLocalLinkTarget("./uconsole/internal/app.go#L42"), "uconsole/internal/app.go#L42"; got != want {
		t.Fatalf("displayLocalLinkTarget() = %q, want %q", got, want)
	}
}

func TestHTMLHeadingLevel(t *testing.T) {
	for tag, want := range map[string]int{
		"h1": 1,
		"h2": 2,
		"h6": 6,
		"p":  6,
	} {
		if got := htmlHeadingLevel(tag); got != want {
			t.Fatalf("htmlHeadingLevel(%q) = %d, want %d", tag, got, want)
		}
	}
}
