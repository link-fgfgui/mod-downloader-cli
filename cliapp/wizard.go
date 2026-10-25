package cliapp

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/cliapp/prompt"
)

// runInstallWizard drives the create-vite-style install flow on a real terminal.
func (r runner) runInstallWizard(ctx context.Context, svc *appcore.Service, runtime runtimeInput, platformFilter string) ([]InstallTarget, error) {
	return r.runInstallWizardWith(ctx, prompt.NewHuh(os.Stdin, r.stdout), svc, runtime, platformFilter)
}

// runInstallWizardWith is the testable wizard core.
func (r runner) runInstallWizardWith(ctx context.Context, p prompt.Prompter, svc *appcore.Service, runtime runtimeInput, platformFilter string) ([]InstallTarget, error) {
	ok, err := p.Confirm(ctx, prompt.ConfirmPrompt{
		Title:       "Install into " + runtime.TargetModsDir + "?",
		Description: "Minecraft " + runtime.MinecraftVersion + " · " + runtime.ModLoader,
		Value:       true,
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, prompt.ErrCanceled
	}

	for attempt := 0; attempt < 3; attempt++ {
		query, err := p.Input(ctx, prompt.InputPrompt{
			Title:       "Search Modrinth",
			Description: "Type a mod name, then choose one or more results",
			Placeholder: "sodium",
			Validate: func(value string) error {
				if strings.TrimSpace(value) == "" {
					return fmt.Errorf("enter a search term")
				}
				return nil
			},
		})
		if err != nil {
			return nil, err
		}
		results := searchWizardProjects(svc, strings.TrimSpace(query), runtime, platformFilter)
		if len(results) == 0 {
			_, _ = fmt.Fprintln(r.stdout, "No matches. Try another search.")
			continue
		}
		options := make([]prompt.Option, 0, len(results))
		byValue := make(map[string]models.ModProject, len(results))
		for _, project := range results {
			value := project.Platform + ":" + firstNonEmptyStr(project.Slug, project.ProjectID)
			label := "[" + project.Platform + "] " + firstNonEmptyStr(project.Title, project.Slug, project.ProjectID)
			desc := firstNonEmptyStr(project.Description, project.ProjectID)
			if len(desc) > 48 {
				desc = desc[:47] + "…"
			}
			options = append(options, prompt.Option{Label: label, Value: value, Description: desc})
			byValue[value] = project
		}
		values, err := p.MultiSelect(ctx, prompt.MultiPrompt{
			Title:       "Select mods to install",
			Description: "Space toggles · Enter confirms",
			Options:     options,
			Min:         1,
		})
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			continue
		}
		confirm, err := p.Confirm(ctx, prompt.ConfirmPrompt{
			Title: fmt.Sprintf("Install %d mod(s)?", len(values)),
			Value: true,
		})
		if err != nil {
			return nil, err
		}
		if !confirm {
			return nil, prompt.ErrCanceled
		}
		refs := make([]string, 0, len(values))
		for _, value := range values {
			if _, ok := byValue[value]; ok {
				refs = append(refs, value)
			}
		}
		targets, err := ParseInstallTargets(refs, "")
		if err != nil {
			return nil, err
		}
		return targets, nil
	}
	return nil, fmt.Errorf("no mods selected")
}

func searchWizardProjects(svc *appcore.Service, query string, runtime runtimeInput, platformFilter string) []models.ModProject {
	update := searchModsTextFn(svc, appstructs.SearchModsRequest{
		Query:     query,
		Version:   runtime.MinecraftVersion,
		ModLoader: runtime.ModLoader,
		Limit:     10,
	})
	out := make([]models.ModProject, 0, len(update.Results))
	for _, project := range update.Results {
		if platformFilter != "" && !strings.EqualFold(project.Platform, platformFilter) {
			continue
		}
		out = append(out, project)
	}
	return out
}
