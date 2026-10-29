package wireguard

import (
	"strings"
	"testing"
	"time"
)

func TestWaitForLinkGivesUpAfterTheTimeout(t *testing.T) {
	const timeout = 300 * time.Millisecond

	start := time.Now()
	err := waitForLink("gate-none0", timeout)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("waitForLink returned nil for a link that never appears")
	}
	if elapsed < timeout {
		t.Errorf("waitForLink gave up after %s, want at least %s", elapsed, timeout)
	}
	if !strings.Contains(err.Error(), "ensure a userspace implementation") {
		t.Errorf("error %q lacks the actionable hint", err)
	}
}
