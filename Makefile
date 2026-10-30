.PHONY: build frontend test test-browser release check-config

NPM ?= npm
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf dev)

frontend:
	cd web && $(NPM) ci --no-fund && $(NPM) run build

build: frontend
	mkdir -p dist
	CGO_ENABLED=0 go build -trimpath -tags production -ldflags '-s -w -X main.version=$(VERSION)' -o dist/opnsense-ip-invites ./cmd/opnsense-ip-invites

test:
	CGO_ENABLED=1 go test -race ./...
	cd web && $(NPM) ci --no-fund && $(NPM) run typecheck && $(NPM) run lint -- --deny-warnings
	$(NPM) ci --no-fund
	$(NPM) test

test-browser:
	cd web && $(NPM) test

release: frontend
	./scripts/build-native.sh '$(VERSION)'

check-config:
	go run ./cmd/opnsense-ip-invites -config examples/config.yaml -check-config
