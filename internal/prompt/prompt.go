package prompt

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

// ErrNotInteractive is returned when a prompt is attempted without a terminal.
var ErrNotInteractive = errors.New("not an interactive terminal")

func theme() *huh.Theme {
	t := huh.ThemeCharm()
	t.Focused.Base = t.Focused.Base.MarginBottom(1)
	t.Blurred.Base = t.Blurred.Base.MarginBottom(1)
	return t
}

// SelectOne draws an interactive single-choice list to stderr and returns the
// index of the chosen option. It returns ErrNotInteractive when stdin is not a
// terminal.
func SelectOne(label string, options []string, active int) (int, error) {
	if len(options) == 0 {
		return -1, errors.New("no options to choose from")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // file descriptors fit in int on every platform Go supports
		return -1, ErrNotInteractive
	}
	opts := make([]huh.Option[int], len(options))
	for i, o := range options {
		display := o
		if i == active {
			display = o + " (current)"
		}
		opts[i] = huh.NewOption(display, i)
	}
	choice := 0
	if active >= 0 && active < len(options) {
		choice = active
	}
	if err := huh.NewSelect[int]().
		Title(label).
		Options(opts...).
		Value(&choice).
		WithTheme(theme()).
		Run(); err != nil {
		return -1, err
	}
	return choice, nil
}

// Confirm draws an interactive yes/no prompt to stderr and returns the answer.
// It returns ErrNotInteractive when stdin is not a terminal.
func Confirm(title string) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // file descriptors fit in int on every platform Go supports
		return false, ErrNotInteractive
	}
	answer := false
	if err := huh.NewConfirm().
		Title(title).
		Value(&answer).
		WithTheme(theme()).
		Run(); err != nil {
		return false, err
	}
	return answer, nil
}

// TypeToConfirm draws an interactive input to stderr and returns nil only when
// the typed text matches expected. It returns ErrNotInteractive when stdin is
// not a terminal.
func TypeToConfirm(title, expected string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // file descriptors fit in int on every platform Go supports
		return ErrNotInteractive
	}
	var typed string
	if err := huh.NewInput().
		Title(title).
		Value(&typed).
		WithTheme(theme()).
		Run(); err != nil {
		return err
	}
	if strings.TrimSpace(typed) != expected {
		return fmt.Errorf("confirmation did not match %q — aborted", expected)
	}
	return nil
}
