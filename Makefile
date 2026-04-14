.PHONY: build install clean test lint fmt vet

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint run

# Binary name and path
BINARY_NAME=llm-wiki
BUILD_DIR=bin
INSTALL_PATH=$(HOME)/go/bin/$(BINARY_NAME)

# Build flags
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X llm-wiki/cmd/llm-wiki/commands.Version=$(VERSION)"

# Default target
all: build

build:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/llm-wiki
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_PATH)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_PATH)"

install: build
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_PATH)

clean:
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	rm -f $(BUILD_DIR)/$(BINARY_NAME)-*

test:
	$(GOTEST) -v -race ./...

lint:
	$(GOLINT) ./...

fmt:
	$(GOFMT) -s -w .

vet:
	$(GOCMD) vet ./...

tidy:
	$(GOMOD) tidy

# Development helpers
dev: tidy build
	@echo "Built at $(BUILD_DIR)/$(BINARY_NAME)"

# Build for multiple platforms
build-all: tidy
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 ./cmd/llm-wiki
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 ./cmd/llm-wiki
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 ./cmd/llm-wiki
	@echo "Binaries built: $(BINARY_NAME)-{linux-amd64,darwin-arm64,darwin-amd64}"
