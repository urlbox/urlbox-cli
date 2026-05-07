// internal/validation/validate_internal_test.go
//
// Internal-package tests for validate.go — exercise unexported helpers that
// can't be reached from package validation_test. Specifically: loadSchemaFrom,
// the testable inner of loadSchema, which lets us cover the three error
// branches (decode/register/compile) that are unreachable through the
// sync.Once-guarded loadSchema entrypoint.
package validation

import (
	"strings"
	"testing"
)

func TestLoadSchemaFrom_InvalidJSON_ReturnsErr(t *testing.T) {
	_, _, err := loadSchemaFrom([]byte("{this is not json"))
	if err == nil {
		t.Fatal("expected error decoding invalid JSON, got nil")
	}
	if !strings.HasPrefix(err.Error(), "decode schema:") {
		t.Errorf("err=%v, want prefix 'decode schema:'", err)
	}
}

func TestLoadSchemaFrom_ValidSchema_Succeeds(t *testing.T) {
	minimal := []byte(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {"foo": {"type": "string"}}
	}`)
	s, keys, err := loadSchemaFrom(minimal)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if s == nil {
		t.Error("schema is nil")
	}
	if len(keys) != 1 || keys[0] != "foo" {
		t.Errorf("keys=%v, want [foo]", keys)
	}
}
