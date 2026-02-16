# Contributing to Koor

Thanks for your interest in contributing to Koor!

## Getting Started

```bash
git clone https://github.com/DavidRHerbert/koor.git
cd koor
go build ./...
go test ./... -v -count=1
```

**Requirements:** Go 1.25+

## Making Changes

1. Fork the repo and create a branch from `main`
2. Make your changes
3. Add or update tests as needed
4. Run `go test ./... -v -count=1` — all tests must pass
5. Run `go vet ./...` — no warnings
6. Submit a pull request

## What to Contribute

- Bug fixes
- Test coverage improvements
- Documentation fixes
- Performance improvements

For new features or architectural changes, please open a Discussion or Issue first to align on approach.

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep changes focused — one PR per concern
- Write tests for new functionality

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
