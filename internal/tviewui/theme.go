package tviewui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Theme identifiers mirror configs.Theme values.
const (
	ThemeDark   = "dark"
	ThemeLight  = "light"
	ThemeSystem = "system"
)

// Theme is the resolved tcell palette used across the TUI.
type Theme struct {
	Name       string
	Primary    tcell.Color
	Secondary  tcell.Color
	Success    tcell.Color
	Warning    tcell.Color
	Error      tcell.Color
	Muted      tcell.Color
	Surface    tcell.Color
	SurfaceAlt tcell.Color
	OnSurface  tcell.Color
	Border     tcell.Color
	SelectedFg tcell.Color
}

func darkTheme() Theme {
	return Theme{
		Name:       ThemeDark,
		Primary:    tcell.PaletteColor(99),
		Secondary:  tcell.PaletteColor(75),
		Success:    tcell.PaletteColor(42),
		Warning:    tcell.PaletteColor(214),
		Error:      tcell.PaletteColor(203),
		Muted:      tcell.PaletteColor(244),
		Surface:    tcell.PaletteColor(236),
		SurfaceAlt: tcell.PaletteColor(238),
		OnSurface:  tcell.PaletteColor(252),
		Border:     tcell.PaletteColor(240),
		SelectedFg: tcell.PaletteColor(230),
	}
}

func lightTheme() Theme {
	return Theme{
		Name:      ThemeLight,
		Primary:   tcell.PaletteColor(25),
		Secondary: tcell.PaletteColor(31),
		Success:   tcell.PaletteColor(28),
		Warning:   tcell.PaletteColor(130),
		Error:     tcell.PaletteColor(160),
		// Muted/border sit on a white surface, so they need real contrast;
		// the near-white greys used before were almost invisible.
		Muted:      tcell.PaletteColor(243),
		Surface:    tcell.PaletteColor(255),
		SurfaceAlt: tcell.PaletteColor(253),
		OnSurface:  tcell.PaletteColor(235),
		Border:     tcell.PaletteColor(245),
		SelectedFg: tcell.PaletteColor(231),
	}
}

// resolveTheme maps a preference to a concrete palette. "system" follows the
// terminal default; tview has no reliable background query, so it resolves to
// the dark palette (the GUI's default).
func resolveTheme(preference string) Theme {
	switch strings.ToLower(strings.TrimSpace(preference)) {
	case ThemeLight:
		return lightTheme()
	case ThemeDark:
		return darkTheme()
	default:
		return darkTheme()
	}
}

// apply installs the palette into tview's global defaults and styles the
// shared primitives. It must run on the UI goroutine.
func (t Theme) apply() {
	tview.Styles.PrimitiveBackgroundColor = t.Surface
	tview.Styles.ContrastBackgroundColor = t.SurfaceAlt
	tview.Styles.MoreContrastBackgroundColor = t.Primary
	tview.Styles.BorderColor = t.Border
	tview.Styles.TitleColor = t.Primary
	tview.Styles.GraphicsColor = t.Primary
	tview.Styles.PrimaryTextColor = t.OnSurface
	tview.Styles.SecondaryTextColor = t.Muted
	tview.Styles.TertiaryTextColor = t.Muted
	tview.Styles.InverseTextColor = t.SelectedFg
	tview.Styles.ContrastSecondaryTextColor = t.OnSurface
}

// selectionStyle is the highlighted row style used by lists and tables.
func (t Theme) selectionStyle() tcell.Style {
	return tcell.StyleDefault.Background(t.Primary).Foreground(t.SelectedFg).Bold(true)
}

// plain is a helper for foreground-only styles.
func (t Theme) plain(color tcell.Color) tcell.Style {
	return tcell.StyleDefault.Foreground(color)
}

// tag returns a tview color tag for c, so TextView content can use the theme.
func (t Theme) tag(c tcell.Color) string {
	hex := c.Hex()
	if hex == 0 {
		return ""
	}
	return fmt.Sprintf("[#%06x]", hex)
}

// tagWith returns a tag plus the given attributes (e.g. "b" for bold).
func (t Theme) tagWith(c tcell.Color, attrs string) string {
	hex := c.Hex()
	if hex == 0 {
		return ""
	}
	if attrs == "" {
		return fmt.Sprintf("[#%06x]", hex)
	}
	return fmt.Sprintf("[#%06x::%s]", hex, attrs)
}

// resetTag ends a color span.
const resetTag = "[-]"

// styleList applies the theme to a navigation/results list.
func (t Theme) styleList(list *tview.List) *tview.List {
	list.SetBackgroundColor(t.Surface)
	// List keeps the text styles captured at construction, so both the
	// foreground and the background have to be set explicitly; changing only
	// the colour leaves the previous palette's background behind.
	list.SetMainTextStyle(tcell.StyleDefault.Foreground(t.OnSurface).Background(t.Surface))
	list.SetSecondaryTextStyle(tcell.StyleDefault.Foreground(t.Muted).Background(t.Surface))
	list.SetSelectedStyle(t.selectionStyle())
	list.SetHighlightFullLine(true)
	return list
}

// styleTable applies the theme to a table.
func (t Theme) styleTable(table *tview.Table) *tview.Table {
	table.SetBackgroundColor(t.Surface)
	table.SetSelectedStyle(t.selectionStyle())
	return table
}

// styleInput applies the theme to an input field.
func (t Theme) styleInput(input *tview.InputField) *tview.InputField {
	input.SetBackgroundColor(t.Surface)
	input.SetFieldBackgroundColor(t.SurfaceAlt)
	input.SetFieldTextColor(t.OnSurface)
	input.SetLabelColor(t.Muted)
	return input
}

// styleButton applies the theme to a button.
func (t Theme) styleButton(button *tview.Button) *tview.Button {
	button.SetStyle(tcell.StyleDefault.Background(t.Surface).Foreground(t.OnSurface))
	button.SetActivatedStyle(t.selectionStyle())
	return button
}

// styleActionButton styles a clickable action-bar chip. The accent colour keeps
// the chips visible against the page background.
func (t Theme) styleActionButton(button *tview.Button) *tview.Button {
	button.SetStyle(tcell.StyleDefault.Background(t.SurfaceAlt).Foreground(t.Primary).Bold(true))
	button.SetActivatedStyle(t.selectionStyle())
	return button
}

// styleTextView applies the palette to a text view. SetBackgroundColor must be
// called on the view itself: TextView keeps the text background in its own text
// style, so styling only its Box leaves stale colours after a theme switch.
func (t Theme) styleTextView(view *tview.TextView) *tview.TextView {
	view.SetBackgroundColor(t.Surface)
	view.SetBorderColor(t.Border)
	view.SetTitleColor(t.Primary)
	view.SetTextColor(t.OnSurface)
	return view
}

// styleBox applies the native border style to a box.
func (t Theme) styleBox(box *tview.Box) *tview.Box {
	box.SetBackgroundColor(t.Surface)
	box.SetBorderColor(t.Border)
	box.SetTitleColor(t.Primary)
	return box
}
