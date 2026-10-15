GO ?= go

.PHONY: build ui ui-check test check clean
build:
	mkdir -p bin
	$(GO) build -trimpath -o bin/pkudisk-fuse ./cmd/pkudisk-fuse
ui:
	cd web && npm ci && npm run build
ui-check:
	cd web && npm run lint && npm run typecheck && npm run test:icons && node scripts/source-hash.mjs --check
test:
	$(GO) test -race ./...
	cd mobile && $(GO) test -race ./...
check:
	$(GO) vet ./...
	cd mobile && $(GO) vet ./...
clean:
	rm -rf bin
