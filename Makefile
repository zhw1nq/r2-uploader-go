# ========================================================
# R2 Uploader Makefile (Production 2026)
# ========================================================

APP_NAME = r2-uploader
BUILD_DIR = bin
ENTRY_POINT = ./cmd/server

.PHONY: all build build-linux build-linux-arm64 build-windows run test vet clean docker-build help

all: vet build

help:
	@echo "Available make targets:"
	@echo "  build              Build binary for current OS in bin/"
	@echo "  build-linux        Cross-compile AMD64 Linux binary in bin/"
	@echo "  build-linux-arm64  Cross-compile ARM64 Linux binary in bin/"
	@echo "  build-windows      Cross-compile Windows 64-bit binary in bin/"
	@echo "  run                Run server in development mode"
	@echo "  test               Run all unit tests"
	@echo "  vet                Run go vet code analysis"
	@echo "  clean              Remove build artifacts in bin/"

# Build for current machine OS
build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) $(ENTRY_POINT)

# Build for 64-bit Linux (Ubuntu, Debian, CentOS, AlmaLinux, etc.)
build-linux:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 $(ENTRY_POINT)

# Build for ARM64 Linux (Raspberry Pi 4/5, AWS Graviton, Apple Silicon VMs)
build-linux-arm64:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 $(ENTRY_POINT)

# Build for Windows 64-bit
build-windows:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME).exe $(ENTRY_POINT)

# Run locally in development mode
run:
	go run $(ENTRY_POINT)

# Run code analysis
vet:
	go vet ./...

# Run unit tests
test:
	go test -v -race ./...

# Clean built binaries
clean:
	rm -rf $(BUILD_DIR)
