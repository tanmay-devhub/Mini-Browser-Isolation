# Contributing

Contributions are welcome. Please follow these guidelines.

## Development setup

```bash
# Run all services locally
make dev

# Run unit tests only (no Docker needed)
make test-unit

# Run e2e tests (requires make dev-detached first)
make test-e2e
```

## Branch and PR workflow

1. Fork the repository and create a branch from `main`.
2. Make your changes with clear, focused commits.
3. Run `go vet ./...` in both `orchestrator/` and `runner/` before pushing.
4. Open a pull request describing what changed and why.

## Code style

- Go: standard `gofmt` formatting, no unused imports.
- TypeScript: existing ESLint config in `frontend/`.
- Commit messages: imperative mood, e.g. `fix: runner context lifecycle`.

## Reporting issues

Open a GitHub issue with:
- Steps to reproduce
- Expected vs actual behaviour
- Relevant logs (`docker logs <container>`)
