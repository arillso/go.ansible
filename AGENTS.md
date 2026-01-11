# GO Ansible

## Context

GO Ansible is a Go module for programmatically executing Ansible playbooks. It provides a clean API to:

- Execute Ansible playbooks from Go applications
- Manage temporary files (SSH keys, Vault passwords) securely
- Install Ansible Galaxy roles and collections
- Build and execute ansible-playbook commands with custom options

The module is designed for embedding Ansible automation within Go applications, particularly useful for infrastructure automation tools.

## Conventions

### Code Style

- Follow standard Go conventions (gofmt, golangci-lint)
- Use context.Context for command execution and cancellation
- Error handling with github.com/pkg/errors for wrapped errors with context
- Struct-based configuration with sensible defaults

### File Organization

- `ansiblePlaybook.go`: Main playbook execution logic and configuration
- `ansiblePlaybook_test.go`: Test suite covering all major functionality
- Minimal dependencies, focus on standard library where possible

### Testing

- All features must have corresponding tests in `ansiblePlaybook_test.go`
- Tests use table-driven patterns where applicable
- Mock external dependencies (file system, command execution) for unit tests

### Configuration

- All Ansible options exposed through `PlaybookConfig` struct
- Builder pattern for setting options
- Validation happens before command execution

## Structure

```
go.ansible/
├── ansiblePlaybook.go       # Main playbook implementation
├── ansiblePlaybook_test.go  # Test suite
├── go.mod                   # Go module dependencies
└── .github/
    └── workflows/
        ├── ci.yml           # Linting
        ├── test.yml         # Tests
        ├── deploy.yml       # Release builds
        ├── codeql.yml       # Security scanning
        └── security.yml     # Trivy scanning
```

## Do Not

- Do not add external dependencies unless absolutely necessary
- Do not modify the public API without considering backwards compatibility
- Do not add OS-specific code without proper build tags
- Do not commit debug code or temporary test files
- Do not add complex abstractions - keep the API simple and focused
