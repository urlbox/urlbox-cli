package prompt

import (
	"errors"
	"testing"
)

func TestTextInput_NonInteractive_ReturnsErrNotInteractive(t *testing.T) {
	_, err := TextInput("Add a short description:", nil)
	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("want ErrNotInteractive, got %v", err)
	}
}
