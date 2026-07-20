# Testing and code quality checks.

[private]
[no-cd]
default: all

# Run all Go tests. `just test` keeps the original top-level command working;
# `just test all` is also available for consistency with other modules.
[no-cd]
all: unit

# Run the test suite.
[no-cd]
unit:
    go test -v ./...

# Regenerate testdata/goldens/*.explain.txt — review the diff before committing, an unreviewed one is a silent regression
[no-cd]
goldens:
    go test ./dialects/... -run TestExplainGoldens -update-goldens -v

# Format code
[no-cd]
fmt:
    go fmt ./...

# Check for suspicious code (built-in Go tool)
[no-cd]
vet:
    go vet ./...

# Lint code (requires golangci-lint)
[no-cd]
lint: lint-seam
    golangci-lint run

# Guard the dialect seam with fragments, not whole words: issue #2 finding 1
# showed that a split literal such as "wiz" + "ard" can evade whole-word
# checks, so these tokens must stay absent from executable Go under internal/
# and cmd/. Dialect data and fixtures remain allowed to name their system.
[no-cd]
lint-seam:
    ! grep -rniE 'wiz|brainyard|sam_harris' --include='*.go' internal/ cmd/

# Run all code quality checks
[no-cd]
check: fmt vet
    @echo "🔍 Running code quality checks..."
    @echo "✅ All checks passed!"

# Run the test suite under the race detector (offline, no backend needed)
[no-cd]
race:
    go test -race ./...
