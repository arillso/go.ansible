# Contributing to go.ansible

Thank you for your interest in contributing! This document provides guidelines and instructions.

## Prerequisites

- Go >= 1.27.4 (the `go` directive in `go.mod`)
- Git
- [lefthook](https://github.com/evilmartians/lefthook) plus the tools its hooks call: Node.js (`npx`), yamllint,
  gitleaks >= 8.18 and actionlint

## Development Setup

### 1. Fork and Clone

```bash
git clone https://github.com/YOUR_USERNAME/go.ansible.git
cd go.ansible
git remote add upstream https://github.com/arillso/go.ansible.git
```

### 2. Install Dependencies and Git Hooks

```bash
go mod download
lefthook install
```

`lefthook install` registers the Git hooks from `lefthook.yml`: pre-commit runs prettier, yamllint, markdownlint,
`gofmt`, `go vet`, actionlint and gitleaks; pre-push runs `go test ./...` and a full gitleaks scan.

### 3. Create a Branch

```bash
git checkout -b feat/your-feature-name
# or
git checkout -b fix/issue-description
```

## Coding Standards

### Go

- Follow standard Go conventions (gofmt, golangci-lint)
- Use `context.Context` for command execution and cancellation
- Error handling with stdlib `errors` and `fmt.Errorf` using `%w`
- Line length: no hard limit, but keep it readable
- Add docstrings for exported functions and types

### YAML

- Use 4 spaces for indentation (no tabs)
- Keep lines under 500 characters
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

Use [Conventional Commits](https://www.conventionalcommits.org/) format:

```text
type(scope): brief summary

Body explaining the why, not the what.
Reference related issues.

Fixes #123
```

Where `type` is one of: `feat`, `fix`, `docs`, `refactor`, `test`, `ci`, `chore`. Mark a breaking change with `!`
after the type or scope (for example `feat(playbook)!: remove GatherSubset`) and a `BREAKING CHANGE:` footer; it ships
in a major version. The pull request title follows the same format, for example
`fix(playbook): pass only flags ansible-playbook supports`.

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
2. Create and push a tag matching `vX.Y.Z` (or `vX.Y.Z-<pre>` for a pre-release); other names do not trigger a
   release:

```bash
git tag v1.2.1
git push origin v1.2.1
```

The `tag.yml` workflow builds and tests the module, then creates a GitHub Release whose notes are the matching
`## [X.Y.Z]` section of `CHANGELOG.md`, plus an SBOM. Without such a section, the release gets GitHub's generated
notes instead.

## Getting Help

- **Issues**: Use GitHub issues for bugs and feature requests
- **Discussions**: Use GitHub Discussions for questions

---

**Thank you for contributing!**
