GO ?= go
NPM ?= npm

.PHONY: build frontend assets test vet e2e archive
build: assets
	mkdir -p bin
	cd backend && $(GO) build -tags production -trimpath -ldflags '-X main.version=local' -o ../bin/support ./cmd/support
frontend:
	cd frontend && $(NPM) ci && $(NPM) run build
assets: frontend
	rm -rf backend/internal/staticui/dist
	mkdir -p backend/internal/staticui/dist
	cp -R frontend/dist/. backend/internal/staticui/dist/
test:
	cd backend && $(GO) test -race ./...
	cd frontend && $(NPM) test
vet:
	cd backend && $(GO) vet ./...
e2e:
	cd frontend && $(NPM) run test:e2e
