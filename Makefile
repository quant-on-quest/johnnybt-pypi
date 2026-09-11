# johnnybt-pypi — private package index
#
#   make            build web UI + Go binary into bin/pypi-server
#   make test       Go unit/integration tests + frontend tests
#   make e2e        real uv publish / uv pip install round-trip
#   make dev        Go server on :8080 (run `make dev-web` in another shell for Vite HMR on :5173)

GO      ?= go
PNPM    ?= pnpm
BIN     := bin/pypi-server
WEB_DIR := web
DIST    := internal/web/dist

.PHONY: all build web web-deps test test-go test-web e2e dev dev-web clean docker

all: build

build: web
	$(GO) build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/pypi-server

## Frontend -------------------------------------------------------------------

web-deps: $(WEB_DIR)/node_modules/.modules.yaml

$(WEB_DIR)/node_modules/.modules.yaml: $(WEB_DIR)/package.json $(WEB_DIR)/pnpm-lock.yaml
	cd $(WEB_DIR) && $(PNPM) install --frozen-lockfile
	@touch $@

web: web-deps
	cd $(WEB_DIR) && $(PNPM) run build
	@touch $(DIST)/.gitkeep

dev-web: web-deps
	cd $(WEB_DIR) && $(PNPM) run dev

## Backend --------------------------------------------------------------------

dev:
	PYPI_ADDR=127.0.0.1:8080 PYPI_DATA_DIR=./data $(GO) run ./cmd/pypi-server

test: test-go test-web

test-go:
	$(GO) vet ./...
	$(GO) test ./...

test-web: web-deps
	cd $(WEB_DIR) && $(PNPM) run test && $(PNPM) run type-check

e2e:
	./scripts/e2e.sh

docker:
	docker build -t johnnybt-pypi:latest .

clean:
	rm -rf bin $(DIST)/assets $(DIST)/index.html
	@touch $(DIST)/.gitkeep
