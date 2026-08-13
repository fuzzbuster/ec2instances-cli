# Repository Guidelines

## Project Structure & Module Organization

This repository is a Go 1.26 CLI-focused fork of `ec2instances.info`. The CLI entry points live in `main.go`, `cli.go`, `providers.go`, and `version.go`. Provider scrapers are organized by top-level provider directories such as `aws/`, `azure/`, `gcp/`, `alicloud/`, `tencentcloud/`, `volcengine/`, `huaweicloud/`, `vultr/`, `linode/`, `digitalocean/`, and `hetzner/`. Shared helpers live in `utils/`; AWS-specific helpers are split between `aws/ec2/` and `aws/awsutils/`. Documentation is in `docs/`, and the reusable agent skill is in `skills/ec2instances/SKILL.md`.

## Build, Test, and Development Commands

- `go build -trimpath -o ec2instances .`: builds the local CLI binary.
- `go test ./...`: runs all package tests.
- `go vet ./...`: checks for common Go correctness issues.
- `test -z "$(gofmt -l .)"`: verifies Go formatting without rewriting files.
- `./ec2instances --json providers`: lists supported providers as one JSON object.
- `./ec2instances --json scrape --providers vultr --output-dir ./output`: runs a targeted scrape; always pass providers explicitly.

## Coding Style & Naming Conventions

Use standard Go formatting via `gofmt`; tabs are expected for indentation in Go files. Keep package names short, lowercase, and aligned with directory names. Prefer clear, semantic identifiers over abbreviations. CLI JSON structs use Go exported field names with explicit `json` tags, for example ``OutputDir string `json:"output_dir,omitempty"` ``. Keep command parsing in the standard library `flag` style already used by `cli.go`.

## Testing Guidelines

Tests use Go's built-in `testing` package and follow `TestName` naming in `*_test.go` files. Add focused unit tests near the package being changed, such as `cli_test.go`, `utils/*_test.go`, or `aws/ec2/*_test.go`. For CLI behavior, prefer testing `runCLI` with buffers instead of shelling out. Run `go test ./...` before submitting changes.

## Commit & Pull Request Guidelines

Recent history follows conventional commit prefixes such as `feat:`, `fix:`, `refactor:`, `docs:`, and `ci:`; scoped variants like `feat(scraper):` are also used. Keep commits atomic and describe the user-visible effect. Pull requests should include a concise summary, the commands run for verification, linked issues when relevant, and sample CLI output or screenshots only when behavior or documentation output changes.

## Security & Configuration Tips

Do not commit cloud credentials or generated scrape output. Required and optional provider environment variables are documented in `README.md`; use local shell environment or CI secrets. Treat JSON output with `partial: true` as incomplete data.
