package tui

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// copyToClipboardCmd writes text to the system clipboard and, additionally,
// emits an OSC 52 escape sequence so the copy also works across SSH sessions
// (where the local clipboard helper is unavailable).
func copyToClipboardCmd(app *AppModel, text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return func() tea.Msg {
		_ = clipboard.WriteAll(text)
		_, _ = fmt.Fprint(os.Stdout, osc52(text))
		return toastMsg{Text: uiText("copy") + " ✓", Color: toastSuccess}
	}
}

// osc52 builds the OSC 52 clipboard-set sequence (base64 payload, ST terminator).
func osc52(text string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	return "\x1b]52;c;" + encoded + "\x07"
}
