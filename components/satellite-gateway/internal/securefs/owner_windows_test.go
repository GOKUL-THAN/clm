package securefs

import (
	"os"
	"testing"
)

func TestWindowsOwnershipChecksFailClosed(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := checkRootOwner(info); err == nil {
		t.Fatal("approved root passed without verifiable Windows ownership")
	}
	if err := checkOwner(info); err == nil {
		t.Fatal("private file passed without verifiable Windows ownership")
	}
}
