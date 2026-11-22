# GO Ansible

[![license](https://img.shields.io/github/license/mashape/apistatus.svg?style=popout-square)](LICENSE)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/arillso/go.ansible?style=popout-square)](https://pkg.go.dev/github.com/arillso/go.ansible/v2?tab=doc)
[![GitHub release](https://img.shields.io/github/v/release/arillso/go.ansible?style=popout-square)](https://github.com/arillso/go.ansible/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/arillso/go.ansible/v2)](https://goreportcard.com/report/github.com/arillso/go.ansible/v2)

A Go module for programmatically executing Ansible playbooks with support for
Galaxy integration, temporary file management, and flexible configuration.

**Documentation:** <https://pkg.go.dev/github.com/arillso/go.ansible/v2>

## Quick Start

Install the module:

```bash
go get github.com/arillso/go.ansible/v2
```

Basic usage:

```go
package main

import (
    "context"
    "log"
    "github.com/arillso/go.ansible/v2" // package name is "ansible"
)

func main() {
    pb := ansible.NewPlaybook()
    pb.Config.Playbooks = []string{"site.yml"}
    pb.Config.Inventories = []string{"localhost,"}

    if err := pb.Exec(context.Background()); err != nil {
        log.Fatalf("Execution failed: %v", err)
    }
}
```

## Examples

Runnable examples live in [`example_test.go`](example_test.go) and on
[pkg.go.dev](https://pkg.go.dev/github.com/arillso/go.ansible/v2#pkg-examples).
A few common setups:

### Vault password

The password is written to a temporary file and passed via
`--vault-password-file`. Set `Config.TempDir` to a tmpfs mount in
security-critical environments so secrets never touch persistent disk.

```go
pb := ansible.NewPlaybook()
pb.Config.Playbooks = []string{"site.yml"}
pb.Config.Inventories = []string{"production,"}
pb.Config.VaultPassword = "s3cr3t"
if err := pb.Exec(context.Background()); err != nil {
    log.Fatal(err)
}
```

### Galaxy requirements

Roles and collections from a requirements file are installed before the run.

```go
pb := ansible.NewPlaybook()
pb.Config.GalaxyFile = "requirements.yml"
pb.Config.Playbooks = []string{"site.yml"}
pb.Config.Inventories = []string{"localhost,"}
if err := pb.Exec(context.Background()); err != nil {
    log.Fatal(err)
}
```

### Extra vars, limit and tags

```go
pb := ansible.NewPlaybook()
pb.Config.Playbooks = []string{"site.yml"}
pb.Config.Inventories = []string{"production,"}
pb.Config.ExtraVars = []string{"env=staging", "version=1.2.3"}
pb.Config.Limit = "web"
pb.Config.Tags = "deploy"
if err := pb.Exec(context.Background()); err != nil {
    log.Fatal(err)
}
```

### Cancellation with context

Cancelling the context terminates the underlying `ansible-playbook` process —
useful for honouring `SIGINT` or an overall deadline.

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
pb := ansible.NewPlaybook()
pb.Config.Playbooks = []string{"site.yml"}
pb.Config.Inventories = []string{"localhost,"}
if err := pb.Exec(ctx); err != nil { // returns a context error when cancelled
    log.Fatal(err)
}
```

Preview the generated command without running Ansible via
`pb.CommandStrings(ctx)`. Options passed as environment variables appear as a
`KEY=VALUE` prefix, e.g. `ANSIBLE_STRATEGY=free ansible-playbook ...`.

## Features

- Automated playbook resolution with glob pattern support
- Secure temporary file management for SSH keys and Vault passwords
- Ansible Galaxy role and collection installation
- Flexible command customization (inventories, extra vars, SSH options)
- Debug mode with command tracing
- Context-based execution with cancellation support

## Configuration

All options are fields on `pb.Config`; each doc comment on
[pkg.go.dev](https://pkg.go.dev/github.com/arillso/go.ansible/v2#Config) says which flag or environment variable it
sets or which temp file or command it triggers. [`docs/option-mapping.md`](docs/option-mapping.md) groups them by
path. Options that behave differently from a plain flag or have a non-zero default:

| Option                                               | Effect                                                                                                                                               | Default (via `NewPlaybook`) |
| ---------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------- |
| `TempDir`                                            | Directory for the temporary SSH key and vault password files (mode `0600`, removed after `Exec`)                                                     | `os.TempDir()`              |
| `PrivateKey`, `VaultPassword`                        | Secret contents, written to a temp file; take precedence over `PrivateKeyFile`/`VaultPasswordFile`; `PrivateKey` must be PEM, otherwise `Exec` fails | empty                       |
| `GalaxyAPIKey`                                       | Passed as `ANSIBLE_GALAXY_TOKEN`, never as a flag                                                                                                    | empty                       |
| `NoColor`                                            | Sets `ANSIBLE_NOCOLOR=1`; without it, color is forced only for an `*os.File` `Stdout` and a `NO_COLOR`/`ANSIBLE_NOCOLOR` that is not true            | `false`                     |
| `ExtraEnv`                                           | Extra environment variables, applied last so they override everything else                                                                           | empty                       |
| `Forks`                                              | `--forks`                                                                                                                                            | `5`                         |
| `GatherSubset`, `GatherTimeout`, `MaxFailPercentage` | Deprecated: `Exec` and `CommandStrings` return an error; set them on the play                                                                        | empty                       |

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). Repository conventions and pitfalls for changes are in
[`AGENTS.md`](AGENTS.md). Run the tests with:

```bash
make test
```

## License

[MIT](LICENSE), (c) 2021-2026, Arillso
