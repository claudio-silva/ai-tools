BINARY  := imagen
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test vet install release clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

install: build
	mkdir -p $(HOME)/bin
	install -m 755 bin/$(BINARY) $(HOME)/bin/$(BINARY)
	@echo
	@echo "Note: if an imagen MCP server is already running (e.g. spawned by your"
	@echo "MCP client), restart the client so it picks up the newly installed binary."

release:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64  go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BINARY)-darwin-arm64  .
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BINARY)-darwin-amd64 .
	lipo -create -output bin/$(BINARY)-darwin-universal bin/$(BINARY)-darwin-arm64 bin/$(BINARY)-darwin-amd64
	# Uncomment when needed:
	# CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64   .
	# CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BINARY)-windows-amd64.exe .

clean:
	rm -rf bin
