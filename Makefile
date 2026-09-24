.PHONY: build clean install install-hev install-claude-capture install-codex-capture vet

PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
HEV_BIN := $(BINDIR)/hev

build:
	go build -o bin/hev ./cmd/hev

clean:
	rm -rf bin/

# Fast path for iteration: builds and drops the hev binary directly into
# $(BINDIR) so a single zsh hash entry always points at the freshly-built
# file. Skips capture setup.
install-hev:
	@go build -o bin/hev ./cmd/hev
	@go run ./cmd/hev install-bin --src bin/hev --dest "$(HEV_BIN)"
	@codesign --force --sign - "$(HEV_BIN)" 2>/dev/null || true
	@echo "Installed hev: $(HEV_BIN)"
	@if [ -n "$$ZSH_VERSION" ] || [ "$${SHELL##*/}" = "zsh" ]; then \
		echo "If 'hev' still runs an older binary, run: hash -r"; \
	fi

install: install-hev
	@"$(HEV_BIN)" config init >/dev/null
	$(MAKE) install-claude-capture
	$(MAKE) install-codex-capture

install-claude-capture:
	@python3 scripts/install-claude-capture.py

install-codex-capture:
	@mkdir -p "$(HOME)/.codex/sessions"
	@echo "Codex rollout capture dir: $(HOME)/.codex/sessions"

vet:
	go vet ./...
