// Package prompt provides create-vite-style interactive prompts for the CLI.
package prompt

import (
	"context"
	"errors"
	"io"

	"github.com/charmbracelet/huh"
)

// ErrCanceled is returned when the user presses Esc or Ctrl-C.
var ErrCanceled = errors.New("canceled")

// Option is one selectable choice.
type Option struct {
	Label       string
	Value       string
	Description string
}

// SelectPrompt describes a single-choice prompt.
type SelectPrompt struct {
	Title       string
	Description string
	Default     string
	Options     []Option
}

// MultiPrompt describes a multi-choice prompt.
type MultiPrompt struct {
	Title       string
	Description string
	Options     []Option
	Min         int
}

// InputPrompt describes a free-text prompt.
type InputPrompt struct {
	Title       string
	Description string
	Placeholder string
	Value       string
	Validate    func(string) error
}

// ConfirmPrompt describes a yes/no prompt.
type ConfirmPrompt struct {
	Title       string
	Description string
	Value       bool
}

// Prompter is the CLI interaction seam used by the install wizard.
type Prompter interface {
	Select(ctx context.Context, p SelectPrompt) (string, error)
	MultiSelect(ctx context.Context, p MultiPrompt) ([]string, error)
	Input(ctx context.Context, p InputPrompt) (string, error)
	Confirm(ctx context.Context, p ConfirmPrompt) (bool, error)
}

// Huh is the Bubble Tea/huh-backed Prompter used on real terminals.
type Huh struct {
	In    io.Reader
	Out   io.Writer
	Width int
}

// NewHuh returns a Prompter writing to out and reading from in.
func NewHuh(in io.Reader, out io.Writer) *Huh {
	return &Huh{In: in, Out: out, Width: 64}
}

func (h *Huh) form(group *huh.Group) *huh.Form {
	f := huh.NewForm(group).WithInput(h.In).WithOutput(h.Out).WithShowHelp(true)
	if h.Width > 0 {
		f = f.WithWidth(h.Width)
	}
	return f
}

func (h *Huh) Select(ctx context.Context, p SelectPrompt) (string, error) {
	value := p.Default
	opts := make([]huh.Option[string], 0, len(p.Options))
	for _, o := range p.Options {
		opt := huh.NewOption(o.Label, o.Value)
		if o.Description != "" {
			opt = huh.NewOption(o.Label+" — "+o.Description, o.Value)
		}
		opts = append(opts, opt)
	}
	field := huh.NewSelect[string]().Title(p.Title).Options(opts...).Value(&value)
	if p.Description != "" {
		field = field.Description(p.Description)
	}
	if err := h.form(huh.NewGroup(field)).RunWithContext(ctx); err != nil {
		return "", mapErr(err)
	}
	return value, nil
}

func (h *Huh) MultiSelect(ctx context.Context, p MultiPrompt) ([]string, error) {
	values := []string{}
	opts := make([]huh.Option[string], 0, len(p.Options))
	for _, o := range p.Options {
		opt := huh.NewOption(o.Label, o.Value)
		if o.Description != "" {
			opt = huh.NewOption(o.Label+" — "+o.Description, o.Value)
		}
		opts = append(opts, opt)
	}
	field := huh.NewMultiSelect[string]().Title(p.Title).Options(opts...).Value(&values)
	if p.Description != "" {
		field = field.Description(p.Description)
	}
	if err := h.form(huh.NewGroup(field)).RunWithContext(ctx); err != nil {
		return nil, mapErr(err)
	}
	return values, nil
}

func (h *Huh) Input(ctx context.Context, p InputPrompt) (string, error) {
	value := p.Value
	field := huh.NewInput().Title(p.Title).Value(&value)
	if p.Placeholder != "" {
		field = field.Placeholder(p.Placeholder)
	}
	if p.Description != "" {
		field = field.Description(p.Description)
	}
	if p.Validate != nil {
		field = field.Validate(p.Validate)
	}
	if err := h.form(huh.NewGroup(field)).RunWithContext(ctx); err != nil {
		return "", mapErr(err)
	}
	return value, nil
}

func (h *Huh) Confirm(ctx context.Context, p ConfirmPrompt) (bool, error) {
	value := p.Value
	field := huh.NewConfirm().Title(p.Title).Value(&value)
	if p.Description != "" {
		field = field.Description(p.Description)
	}
	if err := h.form(huh.NewGroup(field)).RunWithContext(ctx); err != nil {
		return false, mapErr(err)
	}
	return value, nil
}

func mapErr(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrCanceled
	}
	return err
}
