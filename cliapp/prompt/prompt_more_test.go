package prompt

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestOptionChoices(t *testing.T) {
	got := optionChoices([]Option{
		{Label: "A", Value: "a"},
		{Label: "B", Value: "b", Description: "second"},
		{Label: "C", Value: "c", Description: ""},
	})
	want := []struct{ key, value string }{
		{"A", "a"},
		{"B — second", "b"},
		{"C", "c"},
	}
	if len(got) != len(want) {
		t.Fatalf("choices=%d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Key != w.key || got[i].Value != w.value {
			t.Fatalf("choice[%d]=%+v, want key=%q value=%q", i, got[i], w.key, w.value)
		}
	}
	if choices := optionChoices(nil); len(choices) != 0 {
		t.Fatalf("nil options produced %d choices", len(choices))
	}
}

// TestPromptsFailCleanlyWithoutAUsableTerminal exercises every prompt builder
// with input that huh cannot consume. The expected contract is an error (never
// a hang or panic); huh reports ErrTimeout for closed input.
func TestPromptsFailCleanlyWithoutAUsableTerminal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h := NewHuh(strings.NewReader(""), io.Discard)
	selectPrompt := SelectPrompt{
		Title:   "Choose",
		Default: "b",
		Options: []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}},
	}
	if value, err := h.Select(ctx, selectPrompt); err == nil {
		t.Fatalf("Select = %q, nil; want an error", value)
	}

	multiPrompt := MultiPrompt{
		Title:   "Choose many",
		Options: []Option{{Label: "A", Value: "a"}},
	}
	if values, err := h.MultiSelect(ctx, multiPrompt); err == nil {
		t.Fatalf("MultiSelect = %v, nil; want an error", values)
	}

	inputPrompt := InputPrompt{Title: "Name", Value: "prefilled"}
	if value, err := h.Input(ctx, inputPrompt); err == nil {
		t.Fatalf("Input = %q, nil; want an error", value)
	}

	confirmPrompt := ConfirmPrompt{Title: "Confirm", Value: true}
	if value, err := h.Confirm(ctx, confirmPrompt); err == nil {
		t.Fatalf("Confirm = %v, nil; want an error", value)
	}
}

func TestMapErrTimeoutPassesThrough(t *testing.T) {
	err := mapErr(errors.New("timeout"))
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("mapErr lost the underlying error: %v", err)
	}
	if mapped := mapErr(ErrCanceled); !errors.Is(mapped, ErrCanceled) {
		t.Fatalf("mapErr(ErrCanceled) = %v", mapped)
	}
}

func TestHuhImplementsPrompter(t *testing.T) {
	var _ Prompter = (*Huh)(nil)
}

func TestNewHuhDefaults(t *testing.T) {
	in := strings.NewReader("")
	out := io.Discard
	h := NewHuh(in, out)
	if h.In != in || h.Out != out {
		t.Fatal("NewHuh did not store the streams")
	}
	if h.Width != 64 {
		t.Fatalf("Width = %d, want 64", h.Width)
	}
	if !reflect.DeepEqual(h, &Huh{In: in, Out: out, Width: 64}) {
		t.Fatalf("unexpected prompter: %+v", h)
	}
}
