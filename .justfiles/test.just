# Testing and code quality checks.
BUILDTOOL := "go run ./internal/buildutil"

[private]
[no-cd]
default: dashboard

# Run the complete suite, including tests skipped in short mode.
[no-cd]
all: unit

# Run the shared short-mode test dashboard.
[no-cd]
dashboard:
    @{{BUILDTOOL}} test
    # build-util v0.1.0 omits external-test-only packages; keep dialect goldens in this gate.
    go test -short ./dialects/...

# Run the full test suite with raw output (also used by release checks).
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
    @{{BUILDTOOL}} lint

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
