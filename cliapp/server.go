package cliapp

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/link-fgfgui/mod-downloader/core/appcore"
	"github.com/link-fgfgui/mod-downloader/core/httpserver"
	appstructs "github.com/link-fgfgui/mod-downloader/core/structs"
)

// queueModDownloadFn is a seam so tests can assert the server queues resolved
// projects without starting a real download.
var queueModDownloadFn = func(svc *appcore.Service, req appstructs.ModDownloadRequest) appstructs.ModDownloadResult {
	return svc.QueueModDownload(req)
}

// serveInstall starts the core HTTP receiver on httpserver.DefaultAddr and
// blocks until ctx is canceled or the process receives os.Interrupt.
func (r runner) serveInstall(ctx context.Context, svc *appcore.Service, runtime runtimeInput) error {
	var acceptMu sync.Mutex
	accept := func(payloads []appstructs.ExtensionProjectPayload) []appstructs.ModDownloadResult {
		acceptMu.Lock()
		defer acceptMu.Unlock()
		return r.acceptServerPayloads(svc, runtime, payloads)
	}

	server := httpserver.New(httpserver.DefaultAddr, httpserver.Options{Accept: accept})
	if err := server.Start(); err != nil {
		return fmt.Errorf("start http server: %w", err)
	}
	defer server.Stop()

	_, _ = fmt.Fprintf(r.stdout, "serving on http://%s\n", server.Addr())
	_, _ = fmt.Fprintln(r.stdout, `POST a JSON array of {"p":"mr"|"cf","id":...,"slug":...,"file":...} payloads; GET /health for liveness.`)
	_, _ = fmt.Fprintln(r.stdout, "press Ctrl+C to stop")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	defer signal.Stop(stop)

	select {
	case <-ctx.Done():
	case <-stop:
	}
	_, _ = fmt.Fprintln(r.stdout, "stopping http server")
	return nil
}

// acceptServerPayloads resolves each extension payload and queues the resolved
// project immediately. Invalid payloads and resolution failures are reported as
// skipped results in the same order as the request.
func (r runner) acceptServerPayloads(svc *appcore.Service, runtime runtimeInput, payloads []appstructs.ExtensionProjectPayload) []appstructs.ModDownloadResult {
	results := make([]appstructs.ModDownloadResult, len(payloads))
	for i, payload := range payloads {
		results[i] = r.acceptServerPayload(svc, runtime, i, payload)
	}
	return results
}

func (r runner) acceptServerPayload(svc *appcore.Service, runtime runtimeInput, index int, payload appstructs.ExtensionProjectPayload) appstructs.ModDownloadResult {
	platform, ok := serverPlatform(payload.PlatformCode)
	if !ok {
		return appstructs.ModDownloadResult{Skipped: true, Reason: fmt.Sprintf("payload[%d]: invalid platform %q", index, payload.PlatformCode)}
	}

	idOrSlug := ""
	isID := false
	if payload.ID != nil {
		idOrSlug = strings.TrimSpace(*payload.ID)
		isID = idOrSlug != ""
	}
	if idOrSlug == "" {
		idOrSlug = strings.TrimSpace(payload.Slug)
	}
	if idOrSlug == "" {
		return appstructs.ModDownloadResult{Skipped: true, Reason: fmt.Sprintf("payload[%d]: empty project id and slug", index)}
	}

	target := InstallTarget{
		Raw:      platform + ":" + idOrSlug,
		Platform: platform,
		Query:    idOrSlug,
		IsID:     isID,
	}
	projects, err := resolveInstallTarget(svc, target, runtime, platform, false)
	if err != nil {
		return appstructs.ModDownloadResult{Skipped: true, Reason: fmt.Sprintf("payload[%d]: %v", index, err)}
	}
	if len(projects) == 0 {
		return appstructs.ModDownloadResult{Skipped: true, Reason: fmt.Sprintf("payload[%d]: no matching project", index)}
	}

	project := projects[0]
	result := queueModDownloadFn(svc, appstructs.ModDownloadRequest{
		ProjectID:        project.ID,
		Result:           project,
		MinecraftVersion: runtime.MinecraftVersion,
		ModLoader:        runtime.ModLoader,
		VersionID:        strings.TrimSpace(payload.VersionID),
		TargetDir:        runtime.TargetModsDir,
	})
	if result.Queued {
		_, _ = fmt.Fprintf(r.stdout, "queued %s (%s)\n", firstNonEmptyStr(project.Title, project.Slug, project.ID), firstNonEmptyStr(result.FileName, result.VersionID, project.ID))
	}
	return result
}

// serverPlatform maps the browser extension's short platform code to the
// canonical provider platform name.
func serverPlatform(code string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "mr":
		return platformModrinth, true
	case "cf":
		return platformCurseforge, true
	}
	return "", false
}
