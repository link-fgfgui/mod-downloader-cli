package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Fallback terminal geometry used before the first WindowSizeMsg.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

// Theme identifiers mirror configs.Theme values.
const (
	ThemeDark   = "dark"
	ThemeLight  = "light"
	ThemeSystem = "system"
)

// Palette is the resolved color set used across the TUI.
type Palette struct {
	Name       string
	Primary    lipgloss.Color
	Secondary  lipgloss.Color
	Success    lipgloss.Color
	Warning    lipgloss.Color
	Error      lipgloss.Color
	Muted      lipgloss.Color
	Surface    lipgloss.Color
	SurfaceAlt lipgloss.Color
	OnSurface  lipgloss.Color
	Border     lipgloss.Color
}

func darkPalette() Palette {
	return Palette{
		Name:       ThemeDark,
		Primary:    lipgloss.Color("99"),
		Secondary:  lipgloss.Color("75"),
		Success:    lipgloss.Color("42"),
		Warning:    lipgloss.Color("214"),
		Error:      lipgloss.Color("203"),
		Muted:      lipgloss.Color("244"),
		Surface:    lipgloss.Color("236"),
		SurfaceAlt: lipgloss.Color("238"),
		OnSurface:  lipgloss.Color("252"),
		Border:     lipgloss.Color("240"),
	}
}

func lightPalette() Palette {
	return Palette{
		Name:       ThemeLight,
		Primary:    lipgloss.Color("25"),
		Secondary:  lipgloss.Color("31"),
		Success:    lipgloss.Color("28"),
		Warning:    lipgloss.Color("130"),
		Error:      lipgloss.Color("160"),
		Muted:      lipgloss.Color("245"),
		Surface:    lipgloss.Color("254"),
		SurfaceAlt: lipgloss.Color("252"),
		OnSurface:  lipgloss.Color("235"),
		Border:     lipgloss.Color("250"),
	}
}

// ResolvePalette maps a theme preference to a concrete palette. "system" (and
// any unknown value) follows the terminal background, matching the GUI's
// prefers-color-scheme behaviour.
func ResolvePalette(theme string) Palette {
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case ThemeLight:
		return lightPalette()
	case ThemeDark:
		return darkPalette()
	default:
		if lipgloss.HasDarkBackground() {
			return darkPalette()
		}
		return lightPalette()
	}
}

// Theme bundles the resolved palette and pre-built base styles.
type Theme struct {
	Palette Palette
	Simple  bool
}

// NewTheme builds a theme for a preference + simple-mode flag.
func NewTheme(theme string, simple bool) Theme {
	return Theme{Palette: ResolvePalette(theme), Simple: simple}
}

// Reconfigure updates the theme in place.
func (t *Theme) Reconfigure(theme string, simple bool) {
	t.Palette = ResolvePalette(theme)
	t.Simple = simple
}

func (t Theme) Title() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(t.Palette.Primary)
}

func (t Theme) Subtitle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Palette.Muted)
}

func (t Theme) Selected() lipgloss.Style {
	return lipgloss.NewStyle().Background(t.Palette.Primary).Foreground(lipgloss.Color("230")).Bold(true)
}

func (t Theme) Success() lipgloss.Style   { return lipgloss.NewStyle().Foreground(t.Palette.Success) }
func (t Theme) Warning() lipgloss.Style   { return lipgloss.NewStyle().Foreground(t.Palette.Warning) }
func (t Theme) Danger() lipgloss.Style    { return lipgloss.NewStyle().Foreground(t.Palette.Error) }
func (t Theme) Muted() lipgloss.Style     { return lipgloss.NewStyle().Foreground(t.Palette.Muted) }
func (t Theme) Accent() lipgloss.Style    { return lipgloss.NewStyle().Foreground(t.Palette.Secondary) }
func (t Theme) OnSurface() lipgloss.Style { return lipgloss.NewStyle().Foreground(t.Palette.OnSurface) }

func (t Theme) Panel() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Palette.Border).Padding(0, 1)
}

func (t Theme) Overlay() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(t.Palette.Primary).Padding(0, 1)
}

// Bar renders a full-width highlighted line (used for the top/bottom bars).
func (t Theme) Bar() lipgloss.Style {
	return lipgloss.NewStyle().Background(t.Palette.SurfaceAlt).Foreground(t.Palette.OnSurface)
}

// Cursor heights: simple mode makes rows denser.
func (t Theme) RowHeight() int {
	if t.Simple {
		return 1
	}
	return 1
}
