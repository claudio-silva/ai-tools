GO ?= go
VERSION := $(shell cat VERSION)
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := darwin-arm64 darwin-amd64

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o aitools .

test:
	$(GO) test ./...
	$(GO) vet ./...

fmt:
	gofmt -l -w .

# Release binaries and their checksums, as uploaded by `make release`.
dist: $(PLATFORMS:%=dist/aitools-%)
	cd dist && shasum -a 256 aitools-* > checksums.txt

dist/aitools-%:
	GOOS=$(word 1,$(subst -, ,$*)) GOARCH=$(word 2,$(subst -, ,$*)) \
		$(GO) build -ldflags "$(LDFLAGS)" -o $@ .

# Tag v$(VERSION) on the pushed HEAD and publish the GitHub release that
# install.sh downloads from.
release: test
	rm -rf dist
	$(MAKE) dist
	gh release create v$(VERSION) dist/aitools-* dist/checksums.txt \
		--title "aitools $(VERSION)" --generate-notes --target $$(git rev-parse HEAD)

clean:
	rm -rf aitools dist

.PHONY: build test fmt dist release clean
