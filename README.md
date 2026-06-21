# GO Ansible

[![license](https://img.shields.io/github/license/mashape/apistatus.svg?style=popout-square)](LICENSE)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/arillso/go.ansible?style=popout-square)](https://pkg.go.dev/github.com/arillso/go.ansible/v2?tab=doc)
[![GitHub release](https://img.shields.io/github/v/release/arillso/go.ansible?style=popout-square)](https://github.com/arillso/go.ansible/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/arillso/go.ansible/v2)](https://goreportcard.com/report/github.com/arillso/go.ansible/v2)

A Go module for programmatically executing Ansible playbooks with support for Galaxy integration, temporary file management, and flexible configuration.

**Documentation:** https://pkg.go.dev/github.com/arillso/go.ansible/v2

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
err := pb.Exec(context.Background())
```

### Galaxy requirements

Roles and collections from a requirements file are installed before the run.

```go
pb := ansible.NewPlaybook()
pb.Config.GalaxyFile = "requirements.yml"
pb.Config.Playbooks = []string{"site.yml"}
pb.Config.Inventories = []string{"localhost,"}
err := pb.Exec(context.Background())
```

### Extra vars, limit and tags

```go
pb := ansible.NewPlaybook()
pb.Config.Playbooks = []string{"site.yml"}
pb.Config.Inventories = []string{"production,"}
pb.Config.ExtraVars = []string{"env=staging", "version=1.2.3"}
pb.Config.Limit = "web"
pb.Config.Tags = "deploy"
err := pb.Exec(context.Background())
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
err := pb.Exec(ctx) // returns a context error when cancelled
```

Preview the generated command without running Ansible via
`pb.CommandStrings(ctx)`.

## Features

- Automated playbook resolution with glob pattern support
- Secure temporary file management for SSH keys and Vault passwords
- Ansible Galaxy role and collection installation
- Flexible command customization (inventories, extra vars, SSH options)
- Debug mode with command tracing
- Context-based execution with cancellation support

## Testing

Run tests:

```bash
go test -v ./...
```

## License

MIT License

## Copyright

(c) 2021-2026, Arillso
