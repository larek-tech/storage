.PHONY: lint lint-fix install-lint-deps

GOLANG_CI_LINT_VERSION = v2.1.6

lint: install-lint-deps
	golangci-lint run ./...

lint-fix: install-lint-deps
	golangci-lint run --fix ./...

install-lint-deps:
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANG_CI_LINT_VERSION)