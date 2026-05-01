package api_test

import (
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api"
)

func TestEnumValuesFor_WaitUntil_PinsLockedValues(t *testing.T) {
	got := api.EnumValuesFor("wait_until")
	// Pin the four values per the API's locked enum (Phase 4 spec lock).
	// If the schema gains a value, this test fails — update the assertion.
	for _, want := range []string{"domloaded", "mostrequestsfinished", "requestsfinished", "loaded"} {
		if !strings.Contains(got, want) {
			t.Errorf("EnumValuesFor(\"wait_until\")=%q, missing %q", got, want)
		}
	}
}

func TestEnumValuesFor_UnknownField_ReturnsEmpty(t *testing.T) {
	if got := api.EnumValuesFor("not-a-real-field"); got != "" {
		t.Errorf("EnumValuesFor(\"not-a-real-field\")=%q, want empty", got)
	}
}

func TestEnumValuesFor_NonEnumField_ReturnsEmpty(t *testing.T) {
	// `width` is a number; no enum.
	if got := api.EnumValuesFor("width"); got != "" {
		t.Errorf("EnumValuesFor(\"width\")=%q, want empty (no enum on number field)", got)
	}
}
