# Makefile for chaperone authentication gateway

# Binary name
BINARY=chaperone

# Build directory
BUILD_DIR=.

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOFMT=gofmt
GOLINT=golangci-lint

# Build targets
.PHONY: all build test test-race lint fmt clean help

all: build

## build: Build the chaperone binary
build:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY) ./cmd/chaperone

## test: Run all tests
test:
	$(GOTEST) -v ./...

## test-race: Run tests with race detector
test-race:
	$(GOTEST) -race -v ./...

## lint: Run golangci-lint
lint:
	$(GOLINT) run

## fmt: Format all Go source files
fmt:
	$(GOFMT) -w .

## clean: Remove built artifacts
clean:
	$(GOCLEAN)
	rm -f $(BUILD_DIR)/$(BINARY)

## help: Display this help message
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
