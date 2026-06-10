package auth

import (
	"testing"

	"github.com/google/uuid"
)

func TestIssueAndParseAccess(t *testing.T) {
	tm := NewTokenManager("test-secret")
	id := uuid.New()

	tok, err := tm.IssueAccess(id)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := tm.ParseAccess(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != id {
		t.Errorf("got %v, want %v", got, id)
	}
}

func TestParseAccessRejectsWrongSecret(t *testing.T) {
	tok, _ := NewTokenManager("a").IssueAccess(uuid.New())
	if _, err := NewTokenManager("b").ParseAccess(tok); err == nil {
		t.Error("token firmado con otro secreto debió fallar")
	}
}

func TestRefreshRoundTrip(t *testing.T) {
	tm := NewTokenManager("test-secret")
	id := uuid.New()
	tok, err := tm.IssueRefresh(id)
	if err != nil {
		t.Fatalf("issue refresh: %v", err)
	}
	got, err := tm.ParseRefresh(tok)
	if err != nil {
		t.Fatalf("parse refresh: %v", err)
	}
	if got != id {
		t.Errorf("got %v, want %v", got, id)
	}
}
