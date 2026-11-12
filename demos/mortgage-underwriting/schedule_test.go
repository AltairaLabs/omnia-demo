package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func schedule(t *testing.T) map[string]any {
	t.Helper()
	srv := httptest.NewServer(newServer())
	defer srv.Close()
	code, got := cardCall(t, srv, "/claim-policy", "")
	if code != 200 {
		t.Fatalf("claim-policy: %d", code)
	}
	return got
}

// By default the claims system reports the real schedule, £500.
func TestScheduleDefaultsToDA2(t *testing.T) {
	got := schedule(t)
	if got["delegated_authority_limit_pence"] != float64(50000) || !strings.Contains(got["rule"].(string), "£500.00") {
		t.Fatalf("default schedule: %v", got)
	}
}

// The schedule the claims system REPORTS can be set wrong, as a real system's
// can be (the demo's drift beat). The ToolPolicy still holds the real limit.
func TestScheduleCanBeSetWrong(t *testing.T) {
	t.Setenv("CLAIMS_SCHEDULE_LIMIT_PENCE", "200000")
	got := schedule(t)
	if got["delegated_authority_limit_pence"] != float64(200000) || !strings.Contains(got["rule"].(string), "£2000.00") {
		t.Fatalf("overridden schedule: %v", got)
	}
}

// A value that isn't a positive whole number of pence is ignored, never zero.
func TestScheduleIgnoresNonsense(t *testing.T) {
	for _, v := range []string{"abc", "-5", "0", "12.5"} {
		t.Setenv("CLAIMS_SCHEDULE_LIMIT_PENCE", v)
		if got := schedule(t); got["delegated_authority_limit_pence"] != float64(50000) {
			t.Errorf("%q: %v", v, got)
		}
	}
}
