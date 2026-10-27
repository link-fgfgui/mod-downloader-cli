.PHONY: build test test-cli test-core test-all test-race vet golden-update test-live-api test-tui-smoke clean-artifacts clean-user-data

build:
	go build ./cmd/mod-downloader-cli

test: test-cli test-core

test-cli:
	go test ./...

test-core:
	cd core && go test ./...

test-all: test-cli test-core

test-race:
	go test -race ./...
	cd core && go test -race ./...

vet:
	go vet ./...
	cd core && go vet ./...

golden-update:
	go test ./internal/tviewui -run Golden -update

# Real network: Mojang manifest, Modrinth search, version list and a real jar
# download into a temporary instance.
test-live-api:
	MOD_DOWNLOADER_LIVE_API=1 go test ./internal/tviewui -run TestLiveAPI -count=1 -timeout 300s

# Real terminal: build the binary and drive it through a detached tmux PTY.
test-tui-smoke:
	./scripts/tui-smoke.sh

# Remove generated logs and build/test artifacts from the repository root.
# The metadata cache (mod-metadata.tmp) and built binary are safe to recreate.
clean-artifacts:
	rm -f mod-downloader.log mod-downloader.log.[0-9]* mod-metadata.tmp
	rm -f mod-downloader-cli mod-downloader-cli.exe tviewui.test *.test

# Destructive: also removes local favorites/user settings stored in the
# workspace. Run only when the database is known to be disposable.
clean-user-data:
	rm -f mod-favs.sqlite mod-downloader.toml
