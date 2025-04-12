# GO Ansible

<!-- markdownlint-enable -->

[![license](https://img.shields.io/github/license/mashape/apistatus.svg?style=popout-square)](LICENSE)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/arillso/go.ansible?style=popout-square)](https://pkg.go.dev/github.com/arillso/go.ansible?tab=doc)
[![GitHub release](https://img.shields.io/github/v/release/arillso/go.ansible?style=popout-square)](https://github.com/arillso/go.ansible/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/arillso/go.ansible)](https://goreportcard.com/report/github.com/arillso/go.ansible)

<!-- markdownlint-disable -->

## Overview

**GO Ansible** is a Go module designed to programmatically run Ansible playbooks on Linux systems. It supports:

- **Automated Playbook Resolution:** Resolves file patterns (glob) and validates playbook existence.
- **Temporary File Management:** Manages temporary files (SSH private keys and Vault passwords) securely.
- **Galaxy Integration:** Installs roles and collections from Ansible Galaxy with extensive configuration.
- **Flexible Configuration:** Customizes Ansible commands (inventories, extra vars, SSH/user options, verbosity).
- **Enhanced Environment Variables:** Sets variables like `ANSIBLE_CONFIG` based on provided configurations.
- **Debug and Traceability:** Provides detailed command tracing in debug mode for easier troubleshooting.

## Features

### Playbook Execution and Management

- **Playbook Resolution:** Supports file names and glob patterns for playbook identification.
- **Temporary Files:** Manages SSH keys and Vault passwords, cleaning them up automatically.
- **Inventory Management:** Supports inline (`localhost,`) and file-based inventories.

### Advanced Options

- **Command Building:** Constructs commands with options (check/diff modes, user settings, forks).
- **Extra Vars Management:** Passes multiple variables to playbooks with `--extra-vars`.
- **Verbose Logging:** Offers configurable verbosity (up to `-vvvv`) for detailed logs.

### Galaxy Integration

- **Roles and Collections:** Installs Galaxy roles and collections using a configuration file.
- **Customization:** API keys, server URLs, certificate ignoring, timeouts, dependencies handling.
- **Upgrades:** Provides options to upgrade existing Galaxy collections.

### Debugging and Error Handling

- **Command Tracing:** Debug mode prints every executed command for tracing.
- **Context-Based Execution:** Uses Go’s `context.Context` for command management and cancellation.
- **Error Reporting:** Wraps errors with contextual information using `github.com/pkg/errors`.

## Installation

Ensure you have [Go (version 1.23 or higher)](https://golang.org/dl/) installed, then run:

```bash
go mod download
```

## Usage

The module serves as a foundation for executing Ansible playbooks in applications. Example:

1. **Create and Configure a Playbook Instance:**

```go
package main

import (
  "context"
  "log"
  "github.com/arillso/go.ansible/ansible"
)

func main() {
  pb := ansible.NewPlaybook()
  pb.Config.Playbooks = []string{"site.yml"}
  pb.Config.Inventories = []string{"localhost,"}
  pb.Config.PrivateKey = "your ssh private key..."
  pb.Config.VaultPassword = "your vault password..."

  if err := pb.Exec(context.Background()); err != nil {
    log.Fatalf("Execution failed: %v", err)
  }
}
```

2. **Command Construction and Execution:**

The module automatically constructs commands and manages dependencies (like Galaxy installations).

## Testing

The repository includes tests covering functionalities such as playbook resolution, temporary files, and extra vars:

```bash
go test -v ./...
```

## CI/CD and Linters

- **Continuous Integration:** GitHub Actions automatically run tests on push and pull requests.
- **Code Quality:** Makefile targets for formatting and linting ensure best practices.
- **Pre-commit Hooks:** Automatically fix issues like trailing whitespace and line endings.

## License

Licensed under the [MIT License](LICENSE).

## Copyright

(c) 2022, Arillso
