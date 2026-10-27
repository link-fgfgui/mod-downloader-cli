package tviewui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
)

func TestUITextLocalization(t *testing.T) {
	i18n.Apply(i18n.LanguageEnglish)
	if got := uiText("confirm"); got != "Confirm" {
		t.Fatalf("en confirm=%q", got)
	}
	if got := uiText("noInstance"); got != "No instance" {
		t.Fatalf("en noInstance=%q", got)
	}
	i18n.Apply(i18n.LanguageChinese)
	if got := uiText("confirm"); got != "确认" {
		t.Fatalf("zh confirm=%q", got)
	}
	if got := uiText("invalidDirectory"); got != "目录不存在或不是文件夹" {
		t.Fatalf("zh invalidDirectory=%q", got)
	}
	// Unknown keys fall through so a missing chrome label is visible in tests.
	if got := uiText("__does_not_exist"); got != "__does_not_exist" {
		t.Fatalf("unknown key=%q", got)
	}
	i18n.Apply(i18n.LanguageEnglish)
}

func TestChromeLabelsHaveBothLanguages(t *testing.T) {
	for key, entry := range chromeLabels {
		if strings.TrimSpace(entry[0]) == "" || strings.TrimSpace(entry[1]) == "" {
			t.Fatalf("chrome label %q is missing a translation: %#v", key, entry)
		}
	}
}

func TestResolveTheme(t *testing.T) {
	if got := resolveTheme("light"); got.Name != ThemeLight {
		t.Fatalf("light resolved to %q", got.Name)
	}
	if got := resolveTheme(" DARK "); got.Name != ThemeDark {
		t.Fatalf("dark resolved to %q", got.Name)
	}
	for _, value := range []string{"system", "", "unknown"} {
		if got := resolveTheme(value); got.Name != ThemeDark {
			t.Fatalf("%q resolved to %q, want dark fallback", value, got.Name)
		}
	}
	// Light and dark palettes must stay visually distinct.
	if lightTheme().Surface == darkTheme().Surface {
		t.Fatal("light and dark surfaces are identical")
	}
}

func TestThemeTags(t *testing.T) {
	theme := Theme{}
	if got := theme.tag(tcell.ColorRed); got != "[#ff0000]" {
		t.Fatalf("tag=%q", got)
	}
	if got := theme.tagWith(tcell.ColorRed, "b"); got != "[#ff0000::b]" {
		t.Fatalf("tagWith=%q", got)
	}
	if got := theme.tagWith(tcell.ColorRed, ""); got != "[#ff0000]" {
		t.Fatalf("tagWith empty attrs=%q", got)
	}
	fg, _, _ := theme.plain(tcell.ColorRed).Decompose()
	if fg != tcell.ColorRed {
		t.Fatalf("plain style foreground=%v, want red", fg)
	}
}

func TestThemeApplyUsesResolvedPalette(t *testing.T) {
	original := tview.Styles.PrimitiveBackgroundColor
	light := lightTheme()
	light.apply()
	if got := tview.Styles.PrimitiveBackgroundColor; got != light.Surface {
		t.Fatalf("primitive background=%v, want %v", got, light.Surface)
	}
	dark := darkTheme()
	dark.apply()
	if got := tview.Styles.PrimitiveBackgroundColor; got != dark.Surface {
		t.Fatalf("primitive background=%v, want dark", got)
	}
	if original == light.Surface && light.Surface != dark.Surface {
		t.Fatal("theme apply did not change the global palette")
	}
	t.Cleanup(func() { tview.Styles.PrimitiveBackgroundColor = original })
}
