# TUI Test Framework Makefile

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOLINT=golangci-lint

# Build parameters
BINARY_NAME=tuitest
BINARY_UNIX=$(BINARY_NAME)_unix

# Build targets
all: test build

build:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...

test:
	$(GOTEST) -v ./...

test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

test-race:
	$(GOTEST) -v -race ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
	rm -f coverage.out coverage.html

deps:
	$(GOMOD) download
	$(GOMOD) tidy

lint:
	$(GOLINT) run

fmt:
	$(GOCMD) fmt ./...

vet:
	$(GOCMD) vet ./...

# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v

# Development targets
dev-setup: deps
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

validate: fmt vet lint test

# Example targets
run-basic-example:
	$(GOTEST) -v ./examples -run TestBasicExample

run-claude-squad-example:
	$(GOTEST) -v ./integration -run TestClaudeSquadKeyboard

# CI targets
ci: validate test-race test-coverage

.PHONY: all build test test-coverage test-race clean deps lint fmt vet build-linux dev-setup validate ci