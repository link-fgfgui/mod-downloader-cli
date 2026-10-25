package prompt

import (
	"errors"
	"testing"

	"github.com/charmbracelet/huh"
)

func TestMapErrCanceled(t *testing.T) {
	if !errors.Is(mapErr(huh.ErrUserAborted), ErrCanceled) {
		t.Fatal("abort must map to ErrCanceled")
	}
	other := errors.New("boom")
	if !errors.Is(mapErr(other), other) {
		t.Fatal("other errors must pass through")
	}
}

func TestNewHuh(t *testing.T) {
	h := NewHuh(nil, nil)
	if h == nil || h.Width == 0 {
		t.Fatal("huh prompter not configured")
	}
}
