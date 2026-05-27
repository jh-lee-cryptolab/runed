.PHONY: all proto build test clean release-tarball

VERSION ?= v0.1.0-alpha
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

all: build

proto:
	buf generate

build: proto
	mkdir -p bin
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-ldflags "-X main.daemonVersion=$(VERSION)" \
		-o bin/runed ./cmd/runed
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o bin/rundemo ./cmd/rundemo

test:
	go test ./...

clean:
	rm -rf bin/ gen/

release-tarball: build
	mkdir -p dist
	TARNAME=runed-$(VERSION)-$(GOOS)-$(GOARCH).tar.gz; \
	tar -czf dist/$$TARNAME -C bin runed rundemo; \
	cd dist && ( \
		(command -v shasum >/dev/null 2>&1 && shasum -a 256 $$TARNAME > $$TARNAME.sha256) \
		|| (command -v sha256sum >/dev/null 2>&1 && sha256sum $$TARNAME > $$TARNAME.sha256) \
		|| { echo "no sha256 tool available" >&2; exit 1; } \
	); \
	echo "Created: dist/$$TARNAME"
