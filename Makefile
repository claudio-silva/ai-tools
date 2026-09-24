GO ?= go
LDFLAGS := -s -w

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o aitools .

test:
	$(GO) test ./...
	$(GO) vet ./...

fmt:
	gofmt -l -w .

# Build the binaries bundled into the npm package.
dist: dist-darwin-arm64 dist-darwin-amd64

dist-darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o npx/bin/aitools-darwin-arm64 .

dist-darwin-amd64:
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o npx/bin/aitools-darwin-amd64 .

# Build the npm tarball (runs dist first so binaries are fresh).
pack: dist
	cd npx && npm pack

clean:
	rm -f aitools npx/bin/aitools-darwin-* npx/aitools-*.tgz

.PHONY: build test fmt dist dist-darwin-arm64 dist-darwin-amd64 pack clean
