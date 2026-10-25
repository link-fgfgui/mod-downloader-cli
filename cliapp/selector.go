package cliapp

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"

	"github.com/link-fgfgui/mod-downloader-cli/cliapp/prompt"
)

// runCandidateSelector is the create-vite-style picker used for platform
// disambiguation and keyword search results. It replaces the former bespoke
// 400-line Bubble Tea model with huh's Select/MultiSelect.
func runCandidateSelector(title string, projects []models.ModProject, allowMulti bool, svc *appcore.Service, runtime runtimeInput) ([]models.ModProject, bool) {
	if len(projects) == 0 {
		return nil, false
	}
	p := prompt.NewHuh(os.Stdin, os.Stdout)

	options := make([]prompt.Option, 0, len(projects))
	byValue := make(map[string]models.ModProject, len(projects))
	for _, project := range projects {
		value := project.Platform + ":" + firstNonEmptyStr(project.Slug, project.ProjectID)
		label := "[" + project.Platform + "] " + firstNonEmptyStr(project.Title, project.Slug, project.ProjectID)
		desc := firstNonEmptyStr(project.Description, project.ProjectID)
		if len(desc) > 56 {
			desc = desc[:55] + "…"
		}
		options = append(options, prompt.Option{Label: label, Value: value, Description: desc})
		byValue[value] = project
	}

	title = strings.TrimPrefix(strings.TrimSpace(title), "? ")
	var chosen []models.ModProject
	if allowMulti {
		values, err := p.MultiSelect(context.Background(), prompt.MultiPrompt{
			Title:       title,
			Description: "Space toggles · Enter confirms",
			Options:     options,
			Min:         1,
		})
		if err != nil {
			return nil, false
		}
		for _, value := range values {
			if project, ok := byValue[value]; ok {
				chosen = append(chosen, project)
			}
		}
	} else {
		value, err := p.Select(context.Background(), prompt.SelectPrompt{
			Title:   title,
			Options: options,
		})
		if err != nil {
			return nil, false
		}
		if project, ok := byValue[value]; ok {
			chosen = append(chosen, project)
		}
	}
	for _, project := range chosen {
		_, _ = fmt.Fprintf(os.Stdout, "✔ Selected: [%s] %s\n",
			project.Platform, firstNonEmptyStr(project.Title, project.Slug, project.ProjectID))
	}
	return chosen, len(chosen) > 0
}

// humanSize renders a byte count using binary units.
func humanSize(bytes int64) string {
	if bytes <= 0 {
		return "—"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
