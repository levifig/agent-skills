# Loaf development entry points. Everything runs through Go; there is no npm.
#   make build     compile the native binary, regenerate the CLI reference, build content, verify
#   make test      go test ./...
.PHONY: build build-go verify release package test typecheck vet capability-tests clean

GO ?= go

build:
	$(GO) run ./cmd/loafdev build

build-go:
	$(GO) run ./cmd/loafdev build-go

verify:
	$(GO) run ./cmd/loafdev verify-artifacts

release:
	$(GO) run ./cmd/loafdev release

package:
	$(GO) run ./cmd/loafdev package

test:
	$(GO) test ./...

typecheck:
	$(GO) test ./... -run=^$$

vet:
	$(GO) vet ./...

# Harness capability runners still execute under Node; they drive external
# CLIs and read their JSON streams. They need no package install.
capability-tests:
	node --experimental-strip-types --test cli/scripts/smoke-claude-code-startup.test.mjs cli/scripts/smoke-codex-startup.test.mjs cli/scripts/smoke-opencode-request-context.test.mjs cli/scripts/preflight-cursor-agent-context.test.mjs internal/cli/amp_delegation.test.mjs

clean:
	rm -rf bin dist/release
