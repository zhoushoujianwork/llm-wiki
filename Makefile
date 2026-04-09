.PHONY: build install clean test lint fmt vet wiki-install wiki-kill-port wiki-dev wiki-build wiki-preview

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
BUILD_DIR=.
INSTALL_PATH=$(HOME)/go/bin/$(BINARY_NAME)

# Build flags
LDFLAGS=-ldflags "-s -w"

# Default target
all: build

build:
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/llm-wiki

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

# VitePress wiki UI — fixed port 5173 only (requires Node/npm in wiki/)
WIKI_DIR := wiki
WIKI_PORT := 5173

wiki-install:
	cd $(WIKI_DIR) && npm install

# Kill whatever is listening on WIKI_PORT (e.g. old VitePress) so dev never hops to 5174/5175.
wiki-kill-port:
	@for pid in $$(lsof -ti :$(WIKI_PORT) 2>/dev/null); do \
	  echo "Stopping PID $$pid on port $(WIKI_PORT)"; \
	  kill $$pid 2>/dev/null || kill -9 $$pid 2>/dev/null; \
	done

wiki-dev: wiki-install wiki-kill-port
	cd $(WIKI_DIR) && npm run dev -- --port $(WIKI_PORT) --strictPort

wiki-build: wiki-install
	cd $(WIKI_DIR) && npm run build

wiki-preview: wiki-install
	cd $(WIKI_DIR) && npm run preview

# Build for multiple platforms
build-all: tidy
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 ./cmd/llm-wiki
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 ./cmd/llm-wiki
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 ./cmd/llm-wiki
	@echo "Binaries built: $(BINARY_NAME)-{linux-amd64,darwin-arm64,darwin-amd64}"
