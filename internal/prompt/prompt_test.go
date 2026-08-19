package prompt

import (
	"errors"
	"testing"
)

func TestSelectOneNonTTYReturnsErrNotInteractive(t *testing.T) {
	_, err := SelectOne("pick:", []string{"a", "b"}, -1)
	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("want ErrNotInteractive, got %v", err)
	}
}

func TestSelectOneEmptyOptions(t *testing.T) {
	_, err := SelectOne("pick:", nil, -1)
	if err == nil {
		t.Fatal("want error for zero options")
	}
	if errors.Is(err, ErrNotInteractive) {
		t.Fatalf("empty-options must not be reported as not-interactive: %v", err)
	}
	if err.Error() != "no options to choose from" {
		t.Fatalf("want the empty-options error, got %v", err)
	}
}

func TestTypeToConfirmNonTTY(t *testing.T) {
	err := TypeToConfirm("retype:", "expected")
	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("want ErrNotInteractive, got %v", err)
	}
}

func TestConfirmNonTTYReturnsErrNotInteractive(t *testing.T) {
	_, err := Confirm("Switch to this project?")
	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("want ErrNotInteractive, got %v", err)
	}
}
