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
