package agent

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// The hash GetDesiredState returns is compared against the previous hash to
// decide whether the desired state changed (see Run and the watcher loop). It is
// never persisted and never compared across process restarts, so the digest
// algorithm is free to change — but it must stay a stable, content-addressed
// value, which is what these tests pin.

func writeState(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

const validState = `{"ServerPrivateKey":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=","Peers":[]}`

func TestGetDesiredStateHashIsSHA256(t *testing.T) {
	path := writeState(t, validState)

	_, hash, err := GetDesiredState(path)
	if err != nil {
		t.Fatalf("GetDesiredState: %v", err)
	}

	// SHA-256 is 32 bytes, so 64 hex characters. MD5 would be 32 — which is
	// what this asserted against before the algorithm was changed, and is the
	// reason the change is observable rather than cosmetic.
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64 (sha256); got %q", len(hash), hash)
	}
	if _, err := hex.DecodeString(hash); err != nil {
		t.Errorf("hash %q is not hex: %v", hash, err)
	}
}

func TestGetDesiredStateHashIsStable(t *testing.T) {
	path := writeState(t, validState)

	_, first, err := GetDesiredState(path)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	_, second, err := GetDesiredState(path)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}

	if first != second {
		t.Errorf("hash is not stable across reads of identical content: %q != %q", first, second)
	}
}

func TestGetDesiredStateHashChangesWithContent(t *testing.T) {
	// Differs from validState only in the private key, so a hash that ignored
	// content would still collide here.
	const altered = `{"ServerPrivateKey":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb=","Peers":[]}`

	_, a, err := GetDesiredState(writeState(t, validState))
	if err != nil {
		t.Fatalf("reading first fixture: %v", err)
	}
	_, b, err := GetDesiredState(writeState(t, altered))
	if err != nil {
		t.Fatalf("reading second fixture: %v", err)
	}

	if a == b {
		t.Errorf("different content produced the same hash %q — change detection would miss the update", a)
	}
}

func TestGetDesiredStateDecodesState(t *testing.T) {
	// Guards the fix for the digest change against silently breaking the parse:
	// the function returns both, and only the hash is changing.
	state, _, err := GetDesiredState(writeState(t, validState))
	if err != nil {
		t.Fatalf("GetDesiredState: %v", err)
	}
	if got := state.ServerPrivateKey; got != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=" {
		t.Errorf("ServerPrivateKey = %q, want the fixture's key", got)
	}
}

func TestGetDesiredStateReportsMissingFile(t *testing.T) {
	if _, _, err := GetDesiredState(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Error("expected an error for a missing state file, got nil")
	}
}
