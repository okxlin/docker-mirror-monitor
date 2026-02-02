# Docker Mirror Monitor - Makefile
APP_NAME := docker-mirror-monitor
VERSION := 1.0.0
BUILD_TIME := $(shell date +%Y%m%d%H%M%S)
DOCKER_IMAGE := docker-mirror-monitor

# Supported platforms
PLATFORMS := linux/amd64,linux/arm64,linux/arm/v7,linux/ppc64le,linux/s390x

# Go build flags
LDFLAGS := -ldflags="-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

.PHONY: all build clean docker docker-multi run help

help:
	@echo "Usage:"
	@echo "  make build         - Build for current platform"
	@echo "  make build-all     - Build for all platforms"
	@echo "  make docker        - Build Docker image (current arch)"
	@echo "  make docker-multi  - Build multi-arch Docker image"
	@echo "  make run           - Run the application"
	@echo "  make clean         - Clean build artifacts"

# Build for current platform
build:
	go build $(LDFLAGS) -o $(APP_NAME) main.go

# Build for all platforms
build-all: build-linux-amd64 build-linux-arm64 build-linux-armv7 build-linux-ppc64le build-linux-s390x build-windows build-darwin build-darwin-arm64

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(APP_NAME)-linux-amd64 main.go

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(APP_NAME)-linux-arm64 main.go

build-linux-armv7:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build $(LDFLAGS) -o dist/$(APP_NAME)-linux-armv7 main.go

build-linux-ppc64le:
	CGO_ENABLED=0 GOOS=linux GOARCH=ppc64le go build $(LDFLAGS) -o dist/$(APP_NAME)-linux-ppc64le main.go

build-linux-s390x:
	CGO_ENABLED=0 GOOS=linux GOARCH=s390x go build $(LDFLAGS) -o dist/$(APP_NAME)-linux-s390x main.go

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(APP_NAME)-windows-amd64.exe main.go

build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(APP_NAME)-darwin-amd64 main.go

build-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(APP_NAME)-darwin-arm64 main.go

# Build Docker image (single arch)
docker:
	docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .

# Build multi-arch Docker image and push
docker-multi:
	docker buildx build --platform $(PLATFORMS) \
		-t $(DOCKER_IMAGE):$(VERSION) \
		-t $(DOCKER_IMAGE):latest \
		--push .

# Build multi-arch and load locally (only works for single arch at a time)
docker-local:
	docker buildx build --platform linux/amd64 \
		-t $(DOCKER_IMAGE):$(VERSION) \
		-t $(DOCKER_IMAGE):latest \
		--load .

# Run locally
run:
	go run main.go -config config.yaml

# Clean
clean:
	rm -rf dist/
	rm -f $(APP_NAME) $(APP_NAME).exe
