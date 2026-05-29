.PHONY: build install test vet vuln leaks check version

# GitRevolver uses the built-in macOS `security` CLI, so local Darwin builds do
# not need Xcode's cgo toolchain. Override with `CGO_ENABLED=1 make build` if needed.
CGO_ENABLED ?= 0

# Get git info for version
GIT_TAG := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS := -X 'main.version=$(GIT_TAG)' \
           -X 'main.commit=$(GIT_COMMIT)' \
           -X 'main.buildDate=$(BUILD_DATE)'

build:
	@echo "Building gitrevolver..."
	@CGO_ENABLED=$(CGO_ENABLED) go build -ldflags "$(LDFLAGS)" -o gitrevolver .

install:
	@echo "Installing gitrevolver..."
	@CGO_ENABLED=$(CGO_ENABLED) go install -ldflags "$(LDFLAGS)" .

test:
	@CGO_ENABLED=$(CGO_ENABLED) go test ./...

vet:
	@CGO_ENABLED=$(CGO_ENABLED) go vet ./...

vuln:
	@govulncheck ./...

leaks:
	@gitleaks detect --source . --verbose

check: test vet vuln leaks

version:
	@echo "Version: $(GIT_TAG)"
	@echo "Commit: $(GIT_COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
