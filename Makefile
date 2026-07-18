.DEFAULT_GOAL := help

.PHONY: help test

help: ## List commands
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-34s\033[0m %s\n", "make "$$1, $$2}' $(MAKEFILE_LIST)

test: ## Run tests
	@go test $$(go list -f '{{if .TestGoFiles}}{{.ImportPath}}{{end}}' ./...)
