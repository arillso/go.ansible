# Code Review Guidelines

## Scope

In scope:

- Go source changes (`*.go`)
- Test changes (`*_test.go`)
- CI/CD workflow changes
- Renovate configuration updates
- Module metadata (`go.mod`)

Out of scope:

- Renovate dependency-only PRs (patch/minor with automerge enabled)
- Generated changelog entries from release automation

## Required checks

- No secrets committed — no credentials, tokens, or keys in source or tests
- `gofmt` reports no diffs and `golangci-lint` passes
- `go test ./...` passes
- Security scans pass (gitleaks, secretlint)
- Exported API (types, functions, methods) is documented with doc comments

## Severity levels

| Level        | Meaning                                             | Merge impact       |
| ------------ | --------------------------------------------------- | ------------------ |
| Bug          | Incorrect behavior or broken contract               | Blocks merge       |
| Nit          | Minor issue — suboptimal but not incorrect          | Non-blocking       |
| Pre-existing | Issue present before this PR; flagged for awareness | No action required |

## Skip

- Renovate PRs with `automerge: true` (patch/minor) after CI passes
- Documentation-only changes with no functional impact
