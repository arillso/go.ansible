# Contributing to go.ansible

Thank you for your interest in contributing! This document provides guidelines and instructions.

## Prerequisites

- Go >= 1.23
- Git

## Development Setup

### 1. Fork and Clone

```bash
git clone https://github.com/YOUR_USERNAME/go.ansible.git
cd go.ansible
git remote add upstream https://github.com/arillso/go.ansible.git
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Create a Branch

```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/issue-description
```

## Coding Standards

### Go

- Follow standard Go conventions (gofmt, golangci-lint)
- Use `context.Context` for command execution and cancellation
- Error handling with `github.com/pkg/errors` for wrapped errors with context
- Line length: no hard limit, but keep it readable
- Add docstrings for exported functions and types

### YAML

- Use 4 spaces for indentation (no tabs)
- Keep lines under 160 characters
- Require `---` document start

## Testing

### Run Tests

```bash
make test
```

### Linting

```bash
make lint
```

This runs golangci-lint and yamllint.

### Auto-format

```bash
make format
```

### Build

```bash
make build
```

## Submitting Changes

### Commit Messages

Write clear, descriptive commit messages:

```text
Brief summary (50 chars or less)

- Detailed description with bullet points
- Reference related issues

Fixes #123
```

### Pull Request Process

1. Ensure your branch is up to date with `main`
2. Run `make lint` and fix all issues
3. Run `make test` and ensure all tests pass
4. Update `CHANGELOG.md` under `[Unreleased]`
5. Update relevant documentation
6. Create PR and fill out all template sections

### PR Review

- A maintainer will review your PR
- Address any requested changes
- All CI checks must pass
- At least one maintainer approval required

## Release Process

**Note**: Only maintainers can create releases.

1. Update `CHANGELOG.md` - move items from `[Unreleased]` to new version
2. Create and push tag:

```bash
git tag v1.2.1
git push origin v1.2.1
```

GitHub Actions automatically creates a GitHub Release.

## Getting Help

- **Issues**: Use GitHub issues for bugs and feature requests
- **Discussions**: Use GitHub Discussions for questions

---

**Thank you for contributing!**
