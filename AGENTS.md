# GO Ansible

Go library (package `ansible`) that builds and runs `ansible-galaxy` and `ansible-playbook` commands from a `Config`
struct. Consumers embed it, so every exported identifier is public API.

## Conventions

- Every `Config` option is a plain struct field with a doc comment that states how it reaches Ansible
  ("maps to ansible-playbook --x", "sets the ANSIBLE_X environment variable", or the temp file or command it
  triggers). When adding or changing a field, update `docs/option-mapping.md`, which groups the fields by that path.
- Wrap errors with stdlib `fmt.Errorf("...: %w", err)`; `github.com/pkg/errors` was removed as a breaking change
  (see `CHANGELOG.md`).
- A behaviour change gets a test in `ansiblePlaybook_test.go` and an entry under `[Unreleased]` in `CHANGELOG.md`.
  `tag.yml` copies the matching CHANGELOG section into the GitHub Release.
- An option Ansible cannot take stays as a `// Deprecated:` field that `validateUnsupported` rejects
  (see `GatherSubset`), plus a `Deprecated` entry in `CHANGELOG.md`. Removing or renaming an exported field is a
  `BREAKING` entry and a major version.

## Gotchas

- **Flag or environment variable.** `ansible-playbook` rejects unknown flags with `unrecognized arguments`. Before
  adding a flag in `ansibleCommand`, check `ansible-playbook --help`; options without a flag go through
  `buildCustomEnvVars` as `ANSIBLE_*` variables (#133).
- **Play keywords.** Options that exist only as play keywords have neither flag nor variable;
  `validateUnsupported` rejects them so `Exec` fails instead of silently ignoring them.
- **Secrets in output.** `trace` and `CommandStrings` show the `ANSIBLE_*` variables from `Config` before the command
  and mask values of flags and variables listed in `sensitiveFlags`. A new flag or variable that carries a secret goes
  into that map.
- **Temp files.** `PrivateKey` and `VaultPassword` are written to `Config.TempDir` with mode `0600`; `Exec` restores
  the caller's `PrivateKeyFile`/`VaultPasswordFile` afterwards so a reused `Playbook` does not point at a deleted file.
- **Global test state.** `chmodFile` is a package variable that tests swap out. Tests touching it run without
  `t.Parallel()`.
- **Examples are tests.** The `// Output:` lines in `example_test.go` are checked by `go test`; a change to argument
  order or masking updates them.
- **Go directive.** Raising `go` in `go.mod` raises the minimum Go version for every consumer.

## Automation

- `lefthook install` once per clone: pre-commit runs prettier, yamllint, markdownlint, `gofmt`, `go vet`, actionlint
  and gitleaks; pre-push runs `go test ./...` and a full gitleaks scan.
- Pull requests run the `arillso/.github` reusables for Go CI, lint, secret and dependency scans, plus a Claude
  review. Before reviewing or preparing a PR, read `REVIEW.md`; the review job follows it.
- A SemVer tag `vX.Y.Z` triggers `tag.yml`: build, test, GitHub Release and SBOM.
