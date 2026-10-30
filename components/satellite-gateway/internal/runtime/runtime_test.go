package satellite

import (
	"strings"
	"testing"
)

func TestLimitResult(t *testing.T) {
	if got := limitResult("small", 10); got != "small" {
		t.Fatalf("got %q", got)
	}
	got := limitResult(strings.Repeat("x", 100), 32)
	if len(got) > 32 {
		t.Fatalf("result length=%d", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("missing truncation marker: %q", got)
	}
}
