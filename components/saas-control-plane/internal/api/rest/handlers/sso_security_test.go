package handlers

import (
	"testing"

	"github.com/google/uuid"
)

func TestSSOStateIsBoundAndSingleUse(t *testing.T) {
	h := &SSOHandler{states: make(map[string]ssoState)}
	tenantID := uuid.New()
	state := h.generateState(tenantID)
	if state == "" {
		t.Fatal("failed to generate state")
	}

	got, err := h.consumeState(state)
	if err != nil || got != tenantID {
		t.Fatalf("consume state = %v, %v", got, err)
	}
	if _, err := h.consumeState(state); err == nil {
		t.Fatal("state was accepted twice")
	}
}
