package api

import (
	"strings"
	"testing"
)

// White-box tests for extractAPIError covering the Urlbox status-endpoint
// not-found shape: {"renderId": "X", "status": "not-found"}. Without a
// dedicated handler, this body falls through to the generic fallback that
// surfaces the raw JSON as the error message — which is what v1.0.0 did.
// v1.0.1 maps it to a clean "Render X not found" string.

func TestExtractAPIError_StatusNotFoundShape(t *testing.T) {
	body := []byte(`{"renderId":"019e07c0-60b4-70a2","status":"not-found"}`)
	msg, code := extractAPIError(body)
	if !strings.Contains(msg, "019e07c0-60b4-70a2") {
		t.Errorf("msg should reference the renderId; got %q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "not found") {
		t.Errorf("msg should say 'not found'; got %q", msg)
	}
	if strings.Contains(msg, `"renderId"`) {
		t.Errorf("msg should NOT contain the raw JSON body; got %q", msg)
	}
	_ = code // not asserting on code — main bug is the message
}

func TestExtractAPIError_StatusNotFoundShape_NoRenderId(t *testing.T) {
	body := []byte(`{"status":"not-found"}`)
	msg, _ := extractAPIError(body)
	if !strings.Contains(strings.ToLower(msg), "not found") {
		t.Errorf("msg should say 'not found'; got %q", msg)
	}
	if strings.Contains(msg, `"status"`) {
		t.Errorf("msg should NOT contain the raw JSON body; got %q", msg)
	}
}
