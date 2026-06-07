# weft -- web UI to administer an external OpenBSD ldapd(8)
#
# The frontend is built to web/dist and embedded into the Go binary via embed.FS,
# so the release artifact is a single static binary with no runtime dependencies.

BINARY      := weft
PKG         := ./cmd/weft
WEB_DIR     := web
WEB_OUT     := $(WEB_DIR)/dist

GO          ?= go
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)

.PHONY: all build web build-openbsd run test tidy clean fmt vet

all: build

## build: build the binary for the host platform (requires web assets present)
build: web
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)

## build-openbsd: cross-compile a static binary for openbsd/amd64
build-openbsd: web
	GOOS=openbsd GOARCH=amd64 CGO_ENABLED=0 \
		$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY).openbsd-amd64 $(PKG)

## web: build the Svelte SPA into web/dist
web:
	cd $(WEB_DIR) && npm install && npm run build

## run: run the server against ./weft.toml
run:
	$(GO) run $(PKG) -config weft.toml

## test: run Go tests (no external LDAP needed -- uses the Fake directory)
test:
	$(GO) test ./...

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

clean:
	rm -f $(BINARY) $(BINARY).openbsd-amd64
	rm -rf $(WEB_OUT)
