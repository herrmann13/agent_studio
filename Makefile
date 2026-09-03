SHELL := /bin/bash

BUN ?= bun
WAILS ?= $(shell go env GOPATH)/bin/wails
VERSION ?= dev

.DEFAULT_GOAL := help
.PHONY: help setup dev test check build package package-macos package-linux clean

help: ## List the available local development commands.
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

setup: ## Check local tools and install locked frontend and Go dependencies.
	@BUN="$(BUN)" WAILS="$(WAILS)" bash scripts/setup.sh

dev: setup ## Start the desktop app with frontend and Go hot reload.
	@$(WAILS) dev

test: ## Run frontend compilation and Go tests.
	@BUN="$(BUN)" bash scripts/test.sh

check: test ## Run all local validation, including whitespace checks.
	@test -z "$$(gofmt -l .)" || { echo "Run gofmt on:"; gofmt -l .; exit 1; }
	@git diff --check -- . ':(exclude)frontend/wailsjs/go/models.ts'

build: ## Build a production application for the current platform.
	@VERSION="$(VERSION)" WAILS="$(WAILS)" bash scripts/build.sh

package: ## Create the native package for the current platform.
	@case "$$(uname -s)" in Darwin) $(MAKE) package-macos VERSION="$(VERSION)" ;; Linux) $(MAKE) package-linux VERSION="$(VERSION)" ;; *) echo "Unsupported platform: $$(uname -s)"; exit 1 ;; esac

package-macos: ## Create a DMG for the current macOS architecture.
	@VERSION="$(VERSION)" WAILS="$(WAILS)" bash scripts/package-macos.sh

package-linux: ## Create a DEB for the current Linux architecture.
	@VERSION="$(VERSION)" WAILS="$(WAILS)" bash scripts/package-linux.sh

clean: ## Remove locally generated build and package artifacts.
	@rm -rf build/bin dist frontend/dist
