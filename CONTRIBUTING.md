# Contributing to GoTRON MCP

Thank you for your interest in contributing to the GoTRON MCP server.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/<you>/gotron-mcp.git`
3. Create a branch: `git checkout -b feature/my-feature`
4. Make your changes
5. Submit a pull request

## Development Setup

```bash
# Install dependencies
go mod tidy

# Build
make build

# Run tests
make test

# Format code
make fmt

# Lint
make lint
```

## Code Style

- Follow standard Go conventions (`gofmt`, `goimports`, Effective Go)
- Handle all errors explicitly — no blank `_` for error returns
- Use table-driven tests
- Keep functions focused and files small

## Adding a New Tool

1. Create or update the appropriate file in `internal/tools/`
2. Export a `Register*Tools` function following the existing pattern
3. Register it in `internal/server/server.go`
4. Decide if the tool is read-only (both modes) or write (local only)
5. Add tests
6. Update the tool table in `README.md`

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add delegate_resource tool
fix: handle nil account response
docs: update configuration table
test: add TRC20 balance edge cases
```

## Pull Requests

- Keep PRs focused on a single change
- Include tests for new functionality
- Update documentation if adding/changing tools or configuration
- Ensure `make test` and `make lint` pass

## Reporting Issues

Open an issue at [github.com/fbsobreira/gotron-mcp/issues](https://github.com/fbsobreira/gotron-mcp/issues) with:

- Steps to reproduce
- Expected vs actual behavior
- Go version, OS, and network (mainnet/nile/shasta)
