VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

build:
	@go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)" -o keyswift cmd/keyswift/main.go