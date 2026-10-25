package cliapp

import (
	"context"
	"errors"
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/cliapp/prompt"
)

type scriptedPrompter struct {
	confirms []bool
	inputs   []string
	multi    [][]string
	selects  []string
}

func (s *scriptedPrompter) Select(ctx context.Context, p prompt.SelectPrompt) (string, error) {
	if len(s.selects) == 0 {
		return "", prompt.ErrCanceled
	}
	value := s.selects[0]
	s.selects = s.selects[1:]
	return value, nil
}

func (s *scriptedPrompter) MultiSelect(ctx context.Context, p prompt.MultiPrompt) ([]string, error) {
	if len(s.multi) == 0 {
		return nil, nil
	}
	value := s.multi[0]
	s.multi = s.multi[1:]
	return value, nil
}

func (s *scriptedPrompter) Input(ctx context.Context, p prompt.InputPrompt) (string, error) {
	if len(s.inputs) == 0 {
		return "", prompt.ErrCanceled
	}
	value := s.inputs[0]
	s.inputs = s.inputs[1:]
	return value, nil
}

func (s *scriptedPrompter) Confirm(ctx context.Context, p prompt.ConfirmPrompt) (bool, error) {
	if len(s.confirms) == 0 {
		return false, prompt.ErrCanceled
	}
	value := s.confirms[0]
	s.confirms = s.confirms[1:]
	return value, nil
}

func TestRunInstallWizardSelectsProjects(t *testing.T) {
	installSeams(t)
	searchModsTextFn = func(svc *appcore.Service, req appstructs.SearchModsRequest) appstructs.SearchModsUpdate {
		return appstructs.SearchModsUpdate{Results: []models.ModProject{
			projectRef("modrinth", "AANobbMI", "sodium", "Sodium", 1000),
			projectRef("modrinth", "other", "other", "Other", 10),
		}}
	}

	p := &scriptedPrompter{
		confirms: []bool{true, true},
		inputs:   []string{"sodium"},
		multi:    [][]string{{"modrinth:sodium"}},
	}
	runtime := runtimeInput{MinecraftVersion: "1.21.1", ModLoader: "fabric", TargetModsDir: "/tmp/mods"}
	targets, err := (runner{}).runInstallWizardWith(context.Background(), p, nil, runtime, "")
	if err != nil {
		t.Fatalf("wizard: %v", err)
	}
	if len(targets) != 1 || targets[0].Platform != "modrinth" || targets[0].Query != "sodium" {
		t.Fatalf("targets = %+v", targets)
	}
}

func TestRunInstallWizardCancel(t *testing.T) {
	p := &scriptedPrompter{confirms: []bool{false}}
	runtime := runtimeInput{MinecraftVersion: "1.21.1", ModLoader: "fabric", TargetModsDir: "/tmp/mods"}
	_, err := (runner{}).runInstallWizardWith(context.Background(), p, nil, runtime, "")
	if !errors.Is(err, prompt.ErrCanceled) {
		t.Fatalf("err = %v, want ErrCanceled", err)
	}
}
