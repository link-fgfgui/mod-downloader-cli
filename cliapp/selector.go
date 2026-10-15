package cliapp

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
)

// spinnerFrames is the braille spinner used for pending metadata rows.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// errMetadataTimeout flags a candidate whose metadata probe exceeded the budget.
var errMetadataTimeout = fmt.Errorf("metadata timeout")

// selectorCandidate is one selectable row plus its asynchronously enriched
// metadata (Phase 3.1 EnrichedCandidate).
type selectorCandidate struct {
	Project       models.ModProject
	ModIDs        []string
	PrimaryModID  string
	CompatibleVer string
	FileName      string
	FileSize      int64
	LocalState    string
	Loading       bool
	Err           error
	Selected      bool
}

// metadataEnrichedMsg carries one candidate's resolved metadata back to the UI.
type metadataEnrichedMsg struct {
	index         int
	modIDs        []string
	compatibleVer string
	fileName      string
	fileSize      int64
	localState    string
	err           error
}

// selectorSpinnerMsg advances the loading spinner.
type selectorSpinnerMsg struct{}

// InteractiveSelectModel is the inline Bubble Tea selector used for platform
// disambiguation and search-result picking (Phase 4).
type InteractiveSelectModel struct {
	prompt   string
	multi    bool
	items    []selectorCandidate
	cursor   int
	tick     int
	chosen   []int
	done     bool
	canceled bool

	svc     *appcore.Service
	runtime runtimeInput

	// renderedLines tracks the last frame height so the caller can erase it.
	renderedLines int
}

// runCandidateSelector launches the selector inline and returns the chosen
// projects. ok is false when the user skipped (q/Esc) or aborted.
func runCandidateSelector(prompt string, projects []models.ModProject, allowMulti bool, svc *appcore.Service, runtime runtimeInput) ([]models.ModProject, bool) {
	items := make([]selectorCandidate, 0, len(projects))
	for _, project := range projects {
		items = append(items, selectorCandidate{Project: project, Loading: true})
	}
	model := &InteractiveSelectModel{prompt: prompt, multi: allowMulti, items: items, svc: svc, runtime: runtime}

	program := tea.NewProgram(model, tea.WithoutSignalHandler())
	final, err := program.Run()
	if err != nil {
		return nil, false
	}
	result, ok := final.(*InteractiveSelectModel)
	if !ok || result.canceled || len(result.chosen) == 0 {
		clearSelectorFrame(os.Stdout, result)
		return nil, false
	}
	// Clean teardown: erase the inline frame, then leave one ✔ confirmation line.
	clearSelectorFrame(os.Stdout, result)

	out := make([]models.ModProject, 0, len(result.chosen))
	for _, idx := range result.chosen {
		if idx < 0 || idx >= len(result.items) {
			continue
		}
		item := result.items[idx]
		modID := firstNonEmptyStr(item.PrimaryModID, item.Project.Slug, "—")
		version := firstNonEmptyStr(item.CompatibleVer, "—")
		_, _ = fmt.Fprintf(os.Stdout, "✔ Selected: [%s] %s (modid: %s, v%s)\n",
			item.Project.Platform, firstNonEmptyStr(item.Project.Title, item.Project.Slug, item.Project.ProjectID), modID, version)
		out = append(out, item.Project)
	}
	return out, len(out) > 0
}

// clearSelectorFrame erases the selector's rendered lines from the terminal so
// only the caller's confirmation line remains (Phase 4.4 clean teardown).
func clearSelectorFrame(out io.Writer, model *InteractiveSelectModel) {
	if model == nil || model.renderedLines <= 0 {
		return
	}
	_, _ = fmt.Fprintf(out, "\x1b[%dA\x1b[J", model.renderedLines)
}

func (m *InteractiveSelectModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinCmd()}
	for i := range m.items {
		cmds = append(cmds, m.enrichCmd(i))
	}
	return tea.Batch(cmds...)
}

func (m *InteractiveSelectModel) spinCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return selectorSpinnerMsg{} })
}

// enrichCmd performs Phase 3 Steps A–C for one candidate off the UI loop, under
// the per-candidate metadata timeout so a slow provider never stalls the rest.
func (m *InteractiveSelectModel) enrichCmd(index int) tea.Cmd {
	project := m.items[index].Project
	return func() tea.Msg {
		done := make(chan metadataEnrichedMsg, 1)
		go func() {
			msg := metadataEnrichedMsg{index: index, localState: "not_installed"}
			version, status := preferredVersionWithin(m.svc, project, m.runtime, metadataTimeout)
			switch status {
			case versionTimeout:
				done <- metadataEnrichedMsg{index: index, err: errMetadataTimeout, modIDs: slugFallbackModIDs(project), localState: "metadata_timeout"}
				return
			case versionMissing:
				msg.err = fmt.Errorf("no compatible version")
				msg.modIDs = slugFallbackModIDs(project)
				done <- msg
				return
			}
			msg.compatibleVer = firstNonEmptyStr(version.Version, version.Name, version.ID)
			msg.fileName = version.FileName
			msg.fileSize = version.FileSize
			msg.modIDs = resolveModIDs(version, m.runtime.ModLoader)
			if len(msg.modIDs) == 0 {
				msg.modIDs = slugFallbackModIDs(project)
			}
			msg.localState = localStateFor(msg.modIDs, m.runtime)
			done <- msg
		}()
		select {
		case msg := <-done:
			return msg
		case <-time.After(metadataTimeout):
			return metadataEnrichedMsg{index: index, err: errMetadataTimeout, modIDs: slugFallbackModIDs(project), localState: "metadata_timeout"}
		}
	}
}

func (m *InteractiveSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case selectorSpinnerMsg:
		m.tick++
		return m, m.spinCmd()
	case metadataEnrichedMsg:
		if typed.index >= 0 && typed.index < len(m.items) {
			item := &m.items[typed.index]
			item.Loading = false
			item.ModIDs = typed.modIDs
			item.PrimaryModID = firstNonEmptyStr(typed.modIDs...)
			item.CompatibleVer = typed.compatibleVer
			item.FileName = typed.fileName
			item.FileSize = typed.fileSize
			item.LocalState = typed.localState
			item.Err = typed.err
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(typed)
	}
	return m, nil
}

func (m *InteractiveSelectModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c":
		m.canceled = true
		m.done = true
		return m, tea.Quit
	case "q", "esc":
		m.canceled = true
		m.done = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case " ":
		if m.multi && m.cursor < len(m.items) {
			m.items[m.cursor].Selected = !m.items[m.cursor].Selected
		}
	case "enter":
		m.chosen = m.confirmIndices()
		m.done = true
		return m, tea.Quit
	default:
		// 1..9 quick-select.
		if len(key.String()) == 1 && key.String()[0] >= '1' && key.String()[0] <= '9' {
			index := int(key.String()[0] - '1')
			if index < len(m.items) {
				m.cursor = index
				m.chosen = m.confirmIndices()
				m.done = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

// confirmIndices returns the multi-selection, or the cursor row for single mode.
func (m *InteractiveSelectModel) confirmIndices() []int {
	if m.multi {
		indices := []int{}
		for i := range m.items {
			if m.items[i].Selected {
				indices = append(indices, i)
			}
		}
		if len(indices) > 0 {
			return indices
		}
	}
	return []int{m.cursor}
}

func (m *InteractiveSelectModel) View() string {
	styles := selectorStyles()
	width := 100
	var b strings.Builder
	b.WriteString(styles.prompt.Render(m.prompt))
	b.WriteString("\n\n")

	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = styles.cursor.Render("> ")
		}
		check := " "
		if m.multi {
			if item.Selected {
				check = "[✓]"
			} else {
				check = "[ ]"
			}
		}
		title := firstNonEmptyStr(item.Project.Title, item.Project.Slug, item.Project.ProjectID)
		header := fmt.Sprintf("%s%s [%d] [%s] %s (slug: %s)", cursor, check, i+1, item.Project.Platform, title, item.Project.Slug)
		if item.Project.Downloads > 0 {
			header += "  ★ " + compactCount(item.Project.Downloads)
		}
		if i == m.cursor {
			header = styles.selected.Render(stripANSI(header))
		}
		b.WriteString(header)
		b.WriteString("\n")

		// Metadata line(s) with live enrichment.
		spinner := spinnerFrames[m.tick%len(spinnerFrames)]
		meta := "      ├─ "
		if item.Loading {
			meta += "modid: " + spinner + " loading…  |  版本: " + spinner + " 检测中…"
		} else if item.LocalState == "metadata_timeout" {
			meta += styles.accent.Render("modid: " + firstNonEmptyStr(item.PrimaryModID, item.Project.Slug))
			meta += styles.error.Render("  |  版本: metadata_timeout")
		} else if item.Err != nil {
			meta += styles.error.Render("modid: " + firstNonEmptyStr(item.Project.Slug) + "  |  版本: metadata unavailable")
		} else {
			modID := firstNonEmptyStr(item.PrimaryModID, item.Project.Slug)
			meta += "modid: " + styles.accent.Render(modID)
			meta += "  |  版本: " + firstNonEmptyStr(item.CompatibleVer, "—")
			if item.FileSize > 0 {
				meta += "  |  体积: " + humanSize(item.FileSize)
			}
			if item.FileName != "" {
				meta += "  |  " + item.FileName
			}
		}
		b.WriteString(truncateVisible(meta, width))
		b.WriteString("\n")

		state := "[" + localStateLabel(item.LocalState) + "]"
		desc := truncateVisible(item.Project.Description, width-28)
		b.WriteString(styles.muted.Render("      └─ 状态: " + state + "  |  " + desc))
		b.WriteString("\n\n")
	}

	footer := "↑/↓ move · 1-9 select · enter confirm"
	if m.multi {
		footer += " · space toggle"
	}
	footer += " · q/esc skip · ctrl+c abort"
	b.WriteString(styles.footer.Render(footer))

	view := b.String()
	m.renderedLines = lipgloss.Height(view)
	return view
}

func localStateLabel(state string) string {
	switch state {
	case "installed":
		return "已安装"
	case "metadata_timeout":
		return "元数据超时 (modid 回退为 slug)"
	default:
		return "未安装"
	}
}

func selectorStyles() struct {
	prompt   lipgloss.Style
	cursor   lipgloss.Style
	selected lipgloss.Style
	accent   lipgloss.Style
	error    lipgloss.Style
	muted    lipgloss.Style
	footer   lipgloss.Style
} {
	return struct {
		prompt   lipgloss.Style
		cursor   lipgloss.Style
		selected lipgloss.Style
		accent   lipgloss.Style
		error    lipgloss.Style
		muted    lipgloss.Style
		footer   lipgloss.Style
	}{
		prompt:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		cursor:   lipgloss.NewStyle().Foreground(lipgloss.Color("212")),
		selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")),
		accent:   lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		error:    lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		muted:    lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		footer:   lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	}
}

func compactCount(value int64) string {
	if value < 1000 {
		return fmt.Sprintf("%d", value)
	}
	if value >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(value)/1_000_000)
	}
	return fmt.Sprintf("%.1fK", float64(value)/1000)
}

func humanSize(bytes int64) string {
	if bytes <= 0 {
		return "—"
	}
	units := []string{"B", "KiB", "MiB", "GiB"}
	value := float64(bytes)
	idx := 0
	for value >= 1024 && idx < len(units)-1 {
		value /= 1024
		idx++
	}
	if idx == 0 {
		return fmt.Sprintf("%d %s", bytes, units[idx])
	}
	return fmt.Sprintf("%.1f %s", value, units[idx])
}

func truncateVisible(value string, width int) string {
	if width <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width-1]) + "…"
}

func stripANSI(value string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range value {
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
