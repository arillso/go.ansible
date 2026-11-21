// Package ansible provides functionality to run Ansible playbooks with configurable options.
// It supports building command line parameters, handling temporary files, and context-based execution.
package ansible

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	// defaultForks is the default number of parallel processes for Ansible.
	defaultForks = 5

	// maxVerboseLevel is the maximum verbosity level supported by Ansible (-vvvv).
	maxVerboseLevel = 4

	// Ansible exit codes mirror the codes returned by the ansible-playbook
	// CLI. The full set is exported so API consumers can compare against
	// AnsibleError.ExitCode without redefining the magic numbers themselves.
	//
	// ExitCodeSuccess is part of this public set for completeness. The library
	// itself returns nil (not an *AnsibleError carrying this code) on success,
	// so it has no internal use, but it is intentionally exported so consumers
	// have the complete, named set of Ansible exit codes.
	ExitCodeSuccess = 0
	// ExitCodeError is the generic failure code (errors during execution).
	ExitCodeError = 1
	// ExitCodeHostFailed indicates one or more hosts failed during the run.
	ExitCodeHostFailed = 2
	// ExitCodeUnreachable indicates one or more hosts were unreachable.
	ExitCodeUnreachable = 3
	// ExitCodeParserError indicates a playbook or inventory parser error.
	ExitCodeParserError = 4
	// ExitCodeUserAbort indicates the user interrupted execution.
	ExitCodeUserAbort = 99
	// ExitCodeUnexpected indicates an unexpected internal Ansible error.
	ExitCodeUnexpected = 250
)

// AnsibleError represents an error from an Ansible command execution
// with a structured exit code.
type AnsibleError struct {
	// ExitCode is the numeric exit code returned by the Ansible process.
	ExitCode int
	// Command is the name of the command that failed (e.g. "ansible-playbook").
	Command string
	// Message is a human-readable description of the error.
	Message string
	// CommandIndex is the 1-based index of the failed command in the sequence.
	CommandIndex int
	// TotalCommands is the total number of commands in the sequence.
	TotalCommands int
	// Err is the underlying error from exec.Cmd.Run.
	Err error
}

// Error returns a formatted description of the failed Ansible command,
// including the exit code and the command's position in the sequence.
func (e *AnsibleError) Error() string {
	return fmt.Sprintf("%s: %s (exit code %d, command %d/%d)", e.Command, e.Message, e.ExitCode, e.CommandIndex, e.TotalCommands)
}

// Unwrap returns the underlying error from exec.Cmd.Run, allowing use with
// errors.Is and errors.As.
func (e *AnsibleError) Unwrap() error {
	return e.Err
}

// ansibleExitCodeMessage maps Ansible exit codes to human-readable descriptions.
var ansibleExitCodeMessage = map[int]string{
	ExitCodeError:       "general error",
	ExitCodeHostFailed:  "one or more hosts failed",
	ExitCodeUnreachable: "one or more hosts unreachable",
	ExitCodeParserError: "parser error",
	ExitCodeUserAbort:   "user interrupted execution",
	ExitCodeUnexpected:  "unexpected error",
}

// newAnsibleError creates an AnsibleError from an exec.ExitError.
// For non-Ansible commands or non-exit errors it returns nil.
func newAnsibleError(cmdName string, err error) *AnsibleError {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return nil
	}
	code := exitErr.ExitCode()
	msg, ok := ansibleExitCodeMessage[code]
	if !ok {
		msg = "unknown error"
	}
	return &AnsibleError{
		ExitCode: code,
		Command:  cmdName,
		Message:  msg,
		Err:      err,
	}
}

// Config contains configuration options for running Ansible playbooks.
type Config struct {
	// General options

	// Become enables privilege escalation (maps to ansible-playbook --become).
	Become bool
	// BecomeMethod selects the privilege escalation method such as "sudo" or
	// "su" (maps to ansible-playbook --become-method).
	BecomeMethod string
	// BecomeUser is the user to become through privilege escalation
	// (maps to ansible-playbook --become-user).
	BecomeUser string
	// User is the remote user to connect as (maps to ansible-playbook --user).
	User string
	// PrivateKey holds the SSH private key contents. When set, it is written to
	// a temporary file whose path is passed via ansible-playbook --private-key.
	PrivateKey string
	// PrivateKeyFile is the path to the SSH private key file passed via
	// ansible-playbook --private-key. When PrivateKey is set, Exec uses a
	// temporary file instead and leaves this field unchanged.
	PrivateKeyFile string
	// AskBecomePass prompts for the privilege escalation password
	// (maps to ansible-playbook --ask-become-pass).
	AskBecomePass bool
	// AskPass prompts for the connection password
	// (maps to ansible-playbook --ask-pass).
	AskPass bool
	// Check runs in check (dry-run) mode without making changes
	// (maps to ansible-playbook --check).
	Check bool
	// Diff shows the differences when changing files and templates
	// (maps to ansible-playbook --diff).
	Diff bool
	// FlushCache clears the fact cache for every host in the inventory
	// (maps to ansible-playbook --flush-cache).
	FlushCache bool
	// ForceHandlers runs handlers even if a task fails
	// (maps to ansible-playbook --force-handlers).
	ForceHandlers bool
	// SyntaxCheck performs a playbook syntax check without executing it
	// (maps to ansible-playbook --syntax-check).
	SyntaxCheck bool
	// ListHosts lists the hosts that would be targeted, without running tasks
	// (maps to ansible-playbook --list-hosts).
	ListHosts bool
	// ListTags lists all available tags, without running tasks
	// (maps to ansible-playbook --list-tags).
	ListTags bool
	// ListTasks lists all tasks that would be executed, without running them
	// (maps to ansible-playbook --list-tasks).
	ListTasks bool
	// Step prompts for confirmation before running each task
	// (maps to ansible-playbook --step).
	Step bool
	// Connection sets the connection type, e.g. "ssh" or "local"
	// (maps to ansible-playbook --connection).
	Connection string
	// Timeout is the connection timeout in seconds
	// (maps to ansible-playbook --timeout).
	Timeout int
	// Forks is the number of parallel processes to use
	// (maps to ansible-playbook --forks). Defaults to defaultForks.
	Forks int

	// SSH options

	// SSHCommonArgs passes extra arguments to all SSH-based connection tools
	// (maps to ansible-playbook --ssh-common-args).
	SSHCommonArgs string
	// SSHExtraArgs passes extra arguments to the ssh CLI
	// (maps to ansible-playbook --ssh-extra-args).
	SSHExtraArgs string
	// SCPExtraArgs passes extra arguments to the scp CLI
	// (maps to ansible-playbook --scp-extra-args).
	SCPExtraArgs string
	// SFTPExtraArgs passes extra arguments to the sftp CLI
	// (maps to ansible-playbook --sftp-extra-args).
	SFTPExtraArgs string
	// SSHTransferMethod selects the file transfer method, e.g. "smart", "scp"
	// or "sftp" (sets the ANSIBLE_SSH_TRANSFER_METHOD environment variable).
	SSHTransferMethod string

	// Playbook options

	// Inventories is the list of inventory sources. Each entry is passed via a
	// separate ansible-playbook --inventory flag. File-based entries must exist
	// and be readable; inline inventories containing a comma are accepted as-is.
	// When empty, ansible-playbook falls back to its configured inventory
	// (ansible.cfg or ANSIBLE_INVENTORY).
	Inventories []string
	// Playbooks is the list of playbooks to run. Entries may be file paths,
	// glob patterns, or collection FQCN references (namespace.collection.name).
	// They are appended as positional arguments to ansible-playbook.
	Playbooks []string
	// Limit restricts execution to a subset of hosts
	// (maps to ansible-playbook --limit).
	Limit string
	// ExtraVars holds additional variables. Each entry is passed via a separate
	// ansible-playbook --extra-vars flag (key=value or @file).
	ExtraVars []string
	// StartAtTask starts the playbook at the named task
	// (maps to ansible-playbook --start-at-task).
	StartAtTask string
	// Tags runs only tasks tagged with these values
	// (maps to ansible-playbook --tags).
	Tags string
	// SkipTags skips tasks tagged with these values
	// (maps to ansible-playbook --skip-tags).
	SkipTags string
	// ModulePath lists additional directories to search for modules. Each entry
	// is passed via a separate ansible-playbook --module-path flag.
	ModulePath []string
	// Verbose sets the verbosity level (1-4), rendered as -v through -vvvv on
	// both ansible-playbook and ansible-galaxy commands.
	Verbose int
	// NoColor disables colored output: it sets ANSIBLE_NOCOLOR=1 and drops any
	// inherited ANSIBLE_FORCE_COLOR. Without NoColor, color is forced only when
	// Stdout is an *os.File (e.g. the process stdout), not for custom writers.
	NoColor bool

	// Vault options

	// VaultID provides a vault identity (maps to ansible-playbook --vault-id).
	VaultID string
	// VaultPassword holds the vault password contents. When set, it is written
	// to a temporary file whose path is passed via
	// ansible-playbook --vault-password-file.
	VaultPassword string
	// VaultPasswordFile is the path to the vault password file passed via
	// ansible-playbook --vault-password-file. When VaultPassword is set, Exec
	// uses a temporary file instead and leaves this field unchanged.
	VaultPasswordFile string
	// AskVaultPass prompts for the vault password
	// (maps to ansible-playbook --ask-vault-pass).
	AskVaultPass bool

	// Facts options

	// FactPath is the directory of additional local facts
	// (sets the ANSIBLE_FACT_PATH environment variable).
	FactPath string
	// FactCaching selects the fact caching plugin, e.g. "jsonfile" or "redis"
	// (sets the ANSIBLE_FACT_CACHING environment variable).
	FactCaching string
	// FactCachingTimeout is the fact cache expiration in seconds. When greater
	// than zero it sets the ANSIBLE_FACT_CACHING_TIMEOUT environment variable.
	FactCachingTimeout int

	// Galaxy options

	// GalaxyFile is the requirements file for installing roles and collections.
	// When it or GalaxyRequirementsFile is set, ansible-galaxy role install and
	// collection install commands are run before the playbook. It is used as
	// the --role-file and --requirements-file value unless overridden by
	// GalaxyRequirementsFile.
	GalaxyFile string
	// GalaxyAPIKey is the Galaxy API token. It is exported as the
	// ANSIBLE_GALAXY_TOKEN environment variable rather than passed as a flag.
	GalaxyAPIKey string
	// GalaxyAPIServerURL is the Galaxy server URL
	// (maps to ansible-galaxy --server).
	GalaxyAPIServerURL string
	// GalaxyCollectionsPath is the install destination for collections
	// (maps to ansible-galaxy collection install --collections-path).
	GalaxyCollectionsPath string
	// GalaxyDisableGPGVerify disables GPG signature verification of collections
	// (maps to ansible-galaxy collection install --disable-gpg-verify).
	GalaxyDisableGPGVerify bool
	// GalaxyForce forces overwriting of existing roles and collections
	// (maps to ansible-galaxy --force).
	GalaxyForce bool
	// GalaxyForceWithDeps forces reinstallation including all dependencies
	// (maps to ansible-galaxy --force-with-deps).
	GalaxyForceWithDeps bool
	// GalaxyNoDeps disables installation of role dependencies
	// (maps to ansible-galaxy role install --no-deps).
	GalaxyNoDeps bool
	// GalaxyIgnoreCerts skips TLS certificate validation against the server
	// (maps to ansible-galaxy --ignore-certs).
	GalaxyIgnoreCerts bool
	// GalaxyIgnoreSignatureStatusCodes lists gpg status codes to ignore during
	// signature verification. Each entry is passed via a separate
	// ansible-galaxy collection install --ignore-signature-status-code flag.
	GalaxyIgnoreSignatureStatusCodes []string
	// GalaxyKeyring is the keyring used for collection signature verification
	// (maps to ansible-galaxy collection install --keyring).
	GalaxyKeyring string
	// GalaxyOffline installs collections without contacting the Galaxy server
	// (maps to ansible-galaxy collection install --offline).
	GalaxyOffline bool
	// GalaxyPre allows installation of pre-release collections
	// (maps to ansible-galaxy collection install --pre).
	GalaxyPre bool
	// GalaxyRequiredValidSignatureCount is the number of valid collection
	// signatures required (maps to ansible-galaxy collection install
	// --required-valid-signature-count).
	GalaxyRequiredValidSignatureCount int
	// GalaxyRequirementsFile overrides GalaxyFile as the requirements source for
	// both role install (--role-file) and collection install
	// (--requirements-file) when set. It also triggers the galaxy installs on
	// its own. The effective file must exist.
	GalaxyRequirementsFile string
	// GalaxyRolesPath is the install destination for roles
	// (maps to ansible-galaxy role install --roles-path).
	GalaxyRolesPath string
	// GalaxySignature is an additional collection signature source URL
	// (maps to ansible-galaxy collection install --signature).
	GalaxySignature string
	// GalaxyTimeout is the Galaxy server request timeout in seconds
	// (maps to ansible-galaxy --timeout).
	GalaxyTimeout int
	// GalaxyUpgrade upgrades installed collections to the latest version
	// (maps to ansible-galaxy collection install --upgrade).
	GalaxyUpgrade bool

	// Other options

	// CallbacksEnabled enables additional callback plugins
	// (sets the ANSIBLE_CALLBACKS_ENABLED environment variable).
	CallbacksEnabled string
	// PollInterval is the poll interval in seconds for async tasks
	// (sets the ANSIBLE_POLL_INTERVAL environment variable).
	PollInterval int
	// GatherSubset is not supported: ansible-playbook has no CLI flag or
	// environment variable for it. Exec and CommandStrings return an error
	// when it is set.
	//
	// Deprecated: set gather_subset on the play instead.
	GatherSubset string
	// GatherTimeout is not supported: ansible-playbook has no CLI flag or
	// environment variable for it. Exec and CommandStrings return an error
	// when it is set.
	//
	// Deprecated: set gather_timeout on the play instead.
	GatherTimeout int
	// StrategyPlugin selects the execution strategy plugin, e.g. "linear" or
	// "free" (sets the ANSIBLE_STRATEGY environment variable).
	StrategyPlugin string
	// MaxFailPercentage is not supported: ansible-playbook has no CLI flag or
	// environment variable for it. Exec and CommandStrings return an error
	// when it is set.
	//
	// Deprecated: set max_fail_percentage on the play instead.
	MaxFailPercentage int
	// AnyErrorsFatal aborts the entire run if any host fails
	// (sets the ANSIBLE_ANY_ERRORS_FATAL environment variable).
	AnyErrorsFatal bool
	// ConfigFile is the path to an Ansible configuration file
	// (sets the ANSIBLE_CONFIG environment variable).
	// If set, the file must exist or Exec will return an error.
	ConfigFile string

	// OutputCallback sets the ANSIBLE_STDOUT_CALLBACK environment variable,
	// e.g. "json" for machine-readable output or "yaml" for human-friendly output.
	OutputCallback string

	// ExtraEnv holds additional environment variables to pass to Ansible commands.
	// Map keys are variable names and map values are their values.
	// Entries are applied in sorted key order after all other environment variables,
	// so they can override built-in variables such as ANSIBLE_GALAXY_TOKEN (from GalaxyAPIKey).
	ExtraEnv map[string]string

	// ShowVersion runs ansible --version as a separate command before the
	// galaxy and playbook commands.
	ShowVersion bool

	// TempDir is the directory for temporary files (SSH keys, vault passwords).
	// In security-critical environments, set this to a tmpfs or ramfs mount to
	// prevent secrets from persisting on disk if the process is killed (SIGKILL).
	// Defaults to os.TempDir().
	TempDir string
}

// Playbook represents an execution of an Ansible playbook run.
// Playbook is not safe for concurrent use. Create separate instances for concurrent use.
type Playbook struct {
	Config      Config
	Debug       bool      // Enables additional logging output
	TraceOutput io.Writer // Output destination for trace/debug output (defaults to os.Stdout)
	Stdout      io.Writer // Output destination for command stdout (defaults to os.Stdout)
	Stderr      io.Writer // Output destination for command stderr (defaults to os.Stderr)
	tempFiles   []string
}

// NewPlaybook returns a new instance of Playbook with default values.
func NewPlaybook() *Playbook {
	return &Playbook{
		Config: Config{
			Forks:   defaultForks,
			TempDir: os.TempDir(),
		},
	}
}

// Exec runs the configured Ansible playbooks using the provided context.
// It resolves playbook paths, prepares temporary files, builds and executes commands,
// and cleans up temporary files afterward.
func (p *Playbook) Exec(ctx context.Context) error {
	// prepareTempFiles points the *File fields at temp files that are removed
	// afterwards; restore the caller's values so a reused Playbook does not
	// pass a deleted path on the next Exec.
	keyFile, vaultFile := p.Config.PrivateKeyFile, p.Config.VaultPasswordFile
	defer func() {
		p.cleanupTempFiles()
		p.Config.PrivateKeyFile, p.Config.VaultPasswordFile = keyFile, vaultFile
	}()

	if err := p.Config.validateUnsupported(); err != nil {
		return err
	}

	if err := p.resolvePlaybooks(); err != nil {
		return fmt.Errorf("failed to resolve playbooks: %w", err)
	}

	if err := p.prepareTempFiles(); err != nil {
		return fmt.Errorf("failed to prepare temporary files: %w", err)
	}

	cmds, err := p.buildCommands(ctx)
	if err != nil {
		return fmt.Errorf("failed to build commands: %w", err)
	}

	return p.runCommands(ctx, cmds)
}

// validateUnsupported rejects options that ansible-playbook cannot take from
// the command line or the environment; they are play keywords only.
func (c *Config) validateUnsupported() error {
	var unsupported []string
	if c.GatherSubset != "" {
		unsupported = append(unsupported, "GatherSubset (gather_subset)")
	}
	if c.GatherTimeout != 0 {
		unsupported = append(unsupported, "GatherTimeout (gather_timeout)")
	}
	if c.MaxFailPercentage != 0 {
		unsupported = append(unsupported, "MaxFailPercentage (max_fail_percentage)")
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("unsupported options, set them as play keywords instead: %s", strings.Join(unsupported, ", "))
	}
	return nil
}

// isCollectionPlaybook checks if a playbook reference is a collection FQCN.
// Collection playbooks follow the format: namespace.collection.playbook_name
// They must have at least two dots and not be local files or common file patterns.
func isCollectionPlaybook(ref string) bool {
	// If it exists as a file, it's not a collection reference
	if _, err := os.Stat(ref); err == nil {
		return false
	}

	// Check if it matches any files as a glob pattern
	if files, err := filepath.Glob(ref); err == nil && len(files) > 0 {
		return false
	}

	// If it contains common path separators, it's likely a file path
	if strings.Contains(ref, string(os.PathSeparator)) {
		return false
	}

	// Collection FQCNs have at least 2 dots (namespace.collection.playbook)
	dotCount := strings.Count(ref, ".")
	if dotCount < 2 {
		return false
	}

	// Check for common file extensions that indicate it's a file, not a collection
	commonExtensions := []string{".yml", ".yaml", ".json", ".xml"}
	for _, ext := range commonExtensions {
		if strings.HasSuffix(ref, ext) {
			return false
		}
	}

	// At this point, it looks like a collection FQCN
	return true
}

// resolvePlaybooks resolves playbook patterns into concrete file paths and validates their existence.
// It also supports Ansible Collection playbook references in FQCN format (namespace.collection.playbook_name).
func (p *Playbook) resolvePlaybooks() error {
	if len(p.Config.Playbooks) == 0 {
		return errors.New("no playbooks specified")
	}

	var playbooks []string
	for _, pattern := range p.Config.Playbooks {
		// Check if this is a collection playbook reference
		if isCollectionPlaybook(pattern) {
			playbooks = append(playbooks, pattern)
			continue
		}

		// Try to resolve as glob pattern
		if files, err := filepath.Glob(pattern); err == nil && len(files) > 0 {
			for _, file := range files {
				if _, err := os.Stat(file); err == nil {
					playbooks = append(playbooks, file)
				} else {
					return fmt.Errorf("playbook not found: %s: %w", file, err)
				}
			}
		} else if _, err := os.Stat(pattern); err == nil {
			// Try as direct file path
			playbooks = append(playbooks, pattern)
		} else {
			return fmt.Errorf("playbook not found: %s", pattern)
		}
	}

	if len(playbooks) == 0 {
		return errors.New("no playbook files found")
	}

	p.Config.Playbooks = playbooks
	return nil
}

// prepareTempFiles creates necessary temporary files (e.g. private key and vault password)
// and stores their paths for later cleanup.
func (p *Playbook) prepareTempFiles() error {
	if p.Config.PrivateKey != "" {
		file, err := writeTempFile(p.Config.TempDir, "ansible-key-", p.Config.PrivateKey, 0600)
		if err != nil {
			return fmt.Errorf("could not create private key file: %w", err)
		}
		p.Config.PrivateKeyFile = file
		p.tempFiles = append(p.tempFiles, file)
	}
	if p.Config.VaultPassword != "" {
		file, err := writeTempFile(p.Config.TempDir, "ansible-vault-", p.Config.VaultPassword, 0600)
		if err != nil {
			return fmt.Errorf("could not create vault password file: %w", err)
		}
		p.Config.VaultPasswordFile = file
		p.tempFiles = append(p.tempFiles, file)
	}
	return nil
}

// cleanupTempFiles removes all temporary files created during execution.
func (p *Playbook) cleanupTempFiles() {
	for _, f := range p.tempFiles {
		_ = os.Remove(f) // Best effort cleanup, ignore errors
	}
	p.tempFiles = nil
}

// galaxyRequirementsFile returns the effective galaxy requirements file:
// GalaxyRequirementsFile when set, otherwise GalaxyFile.
func (c *Config) galaxyRequirementsFile() string {
	if c.GalaxyRequirementsFile != "" {
		return c.GalaxyRequirementsFile
	}
	return c.GalaxyFile
}

// buildCommands constructs the list of exec.Cmd commands (version, galaxy, playbook)
// based on the configuration and given context.
func (p *Playbook) buildCommands(ctx context.Context) ([]*exec.Cmd, error) {
	var cmds []*exec.Cmd

	// Version command (optional)
	if p.Config.ShowVersion {
		cmds = append(cmds, p.versionCommand(ctx))
	}

	// Galaxy commands (if GalaxyFile or GalaxyRequirementsFile is set)
	if reqFile := p.Config.galaxyRequirementsFile(); reqFile != "" {
		if _, err := os.Stat(reqFile); err != nil {
			return nil, fmt.Errorf("galaxy file not found: %s: %w", reqFile, err)
		}
		cmds = append(cmds, p.galaxyRoleCommand(ctx), p.galaxyCollectionCommand(ctx))
	}

	// Validate all inventories
	for _, inv := range p.Config.Inventories {
		if err := validateInventory(inv); err != nil {
			return nil, err
		}
	}

	// The playbook always runs. Without inventories ansible-playbook uses its
	// configured inventory (ansible.cfg or ANSIBLE_INVENTORY).
	cmds = append(cmds, p.ansibleCommand(ctx, p.Config.Inventories))

	return cmds, nil
}

// CommandStrings returns the command lines that would be executed, without running them.
// This is useful for debugging, logging, or previewing the commands.
// Values of sensitive flags (--extra-vars, --private-key, --vault-password-file)
// are masked as in debug trace output; no temporary files are written.
// This method works on a shallow copy of the Playbook so it does not mutate Config.
func (p *Playbook) CommandStrings(ctx context.Context) ([]string, error) {
	// Work on a shallow copy so resolvePlaybooks does not mutate the caller's Config.
	cp := *p
	cp.Config.Playbooks = make([]string, len(p.Config.Playbooks))
	copy(cp.Config.Playbooks, p.Config.Playbooks)

	if err := cp.Config.validateUnsupported(); err != nil {
		return nil, err
	}

	// Exec passes temp files for inline secrets; show the flags with a
	// placeholder (masked below) instead of writing secrets to disk.
	if cp.Config.PrivateKey != "" {
		cp.Config.PrivateKeyFile = "******"
	}
	if cp.Config.VaultPassword != "" {
		cp.Config.VaultPasswordFile = "******"
	}

	if err := cp.resolvePlaybooks(); err != nil {
		return nil, fmt.Errorf("failed to resolve playbooks: %w", err)
	}

	cmds, err := cp.buildCommands(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build commands: %w", err)
	}

	result := make([]string, len(cmds))
	for i, cmd := range cmds {
		result[i] = strings.Join(maskSensitiveArgs(cmd.Args), " ")
	}
	return result, nil
}

// commandEnv builds the environment shared by all commands of one run.
func (p *Playbook) commandEnv(stdout io.Writer, envVars []string) []string {
	inherited := os.Environ()
	env := make([]string, 0, len(inherited)+len(envVars)+len(p.Config.ExtraEnv)+3)
	for _, kv := range inherited {
		// NoColor must win over an inherited ANSIBLE_FORCE_COLOR.
		if p.Config.NoColor && strings.HasPrefix(kv, "ANSIBLE_FORCE_COLOR=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "ANSIBLE_GALAXY_DISPLAY_PROGRESS=0")
	if p.Config.NoColor {
		env = append(env, "ANSIBLE_NOCOLOR=1")
	} else if _, isFile := stdout.(*os.File); isFile {
		// ponytail: *os.File, not a TTY check; keeps color in CI logs (pipes)
		// while custom writers such as bytes.Buffer get plain text.
		env = append(env, "ANSIBLE_FORCE_COLOR=1")
	}
	if p.Config.GalaxyAPIKey != "" {
		env = append(env, "ANSIBLE_GALAXY_TOKEN="+p.Config.GalaxyAPIKey)
	}
	env = append(env, envVars...)
	envKeys := make([]string, 0, len(p.Config.ExtraEnv))
	for k := range p.Config.ExtraEnv {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		env = append(env, k+"="+p.Config.ExtraEnv[k])
	}
	return env
}

// runCommands executes the given commands sequentially.
// The context is already embedded in each exec.Cmd via exec.CommandContext;
// it is used here only to attach ctx.Err() to the error of a killed command.
func (p *Playbook) runCommands(ctx context.Context, cmds []*exec.Cmd) error {
	envVars, err := buildCustomEnvVars(&p.Config)
	if err != nil {
		return fmt.Errorf("failed to build environment: %w", err)
	}
	stdout := p.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := p.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	env := p.commandEnv(stdout, envVars)
	for i, cmd := range cmds {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		cmd.Env = env
		if p.Debug {
			p.trace(cmd)
		}
		if err := cmd.Run(); err != nil {
			// A cancelled context kills the process; keep ctx.Err() in the
			// chain so callers can match context.Canceled/DeadlineExceeded.
			// Start already returns ctx.Err() when ctx was done beforehand.
			if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(err, ctxErr) {
				err = fmt.Errorf("%w: %w", ctxErr, err)
			}
			cmdName := filepath.Base(cmd.Path)
			if ae := newAnsibleError(cmdName, err); ae != nil {
				ae.CommandIndex = i + 1
				ae.TotalCommands = len(cmds)
				return ae
			}
			return fmt.Errorf("error executing %s (command %d/%d): %w", cmdName, i+1, len(cmds), err)
		}
	}
	return nil
}

// validateInventory checks whether the inventory file or directory exists and
// is readable. For inline inventories (containing a comma), it is assumed to be valid.
func validateInventory(inv string) error {
	if strings.Contains(inv, ",") {
		return nil
	}
	fi, err := os.Stat(inv)
	if os.IsNotExist(err) {
		return fmt.Errorf("inventory not found: %s", inv)
	}
	if err != nil {
		return fmt.Errorf("inventory not readable: %s: %w", inv, err)
	}
	// Opening a FIFO or device can block; only probe files and directories.
	if !fi.Mode().IsRegular() && !fi.IsDir() {
		return nil
	}
	f, err := os.Open(inv) //nolint:gosec // path comes from trusted configuration
	if err != nil {
		return fmt.Errorf("inventory not readable: %s: %w", inv, err)
	}
	_ = f.Close()
	return nil
}

// buildCustomEnvVars constructs additional environment variables for Ansible.
// It returns an error if ConfigFile is set but does not exist.
func buildCustomEnvVars(cfg *Config) ([]string, error) {
	var env []string
	if cfg.ConfigFile != "" {
		if _, err := os.Stat(cfg.ConfigFile); err != nil {
			return nil, fmt.Errorf("config file not found: %s", cfg.ConfigFile)
		}
		env = append(env, "ANSIBLE_CONFIG="+cfg.ConfigFile)
	}
	if cfg.FactPath != "" {
		env = append(env, "ANSIBLE_FACT_PATH="+cfg.FactPath)
	}
	if cfg.FactCaching != "" {
		env = append(env, "ANSIBLE_FACT_CACHING="+cfg.FactCaching)
	}
	if cfg.FactCachingTimeout > 0 {
		env = append(env, "ANSIBLE_FACT_CACHING_TIMEOUT="+strconv.Itoa(cfg.FactCachingTimeout))
	}
	if cfg.OutputCallback != "" {
		env = append(env, "ANSIBLE_STDOUT_CALLBACK="+cfg.OutputCallback)
	}
	// ansible-playbook has no CLI flags for these; they are config settings.
	if cfg.StrategyPlugin != "" {
		env = append(env, "ANSIBLE_STRATEGY="+cfg.StrategyPlugin)
	}
	if cfg.CallbacksEnabled != "" {
		env = append(env, "ANSIBLE_CALLBACKS_ENABLED="+cfg.CallbacksEnabled)
	}
	if cfg.PollInterval > 0 {
		env = append(env, "ANSIBLE_POLL_INTERVAL="+strconv.Itoa(cfg.PollInterval))
	}
	if cfg.AnyErrorsFatal {
		env = append(env, "ANSIBLE_ANY_ERRORS_FATAL=True")
	}
	if cfg.SSHTransferMethod != "" {
		env = append(env, "ANSIBLE_SSH_TRANSFER_METHOD="+cfg.SSHTransferMethod)
	}
	return env, nil
}

// argOption represents a single command-line flag option.
type argOption struct {
	flag  string
	value any
}

// applyOption appends the flag and its value (if set) to the args slice.
func applyOption(args []string, opt argOption) []string {
	switch v := opt.value.(type) {
	case string:
		if v != "" {
			args = append(args, opt.flag, v)
		}
	case int:
		if v != 0 {
			args = append(args, opt.flag, strconv.Itoa(v))
		}
	case bool:
		if v {
			args = append(args, opt.flag)
		}
	case []string:
		if len(v) > 0 {
			for _, item := range v {
				if item != "" {
					args = append(args, opt.flag, item)
				}
			}
		}
	}
	return args
}

// addVerbose appends a verbose flag (e.g. "-vv") based on the level.
func addVerbose(args []string, level int) []string {
	if level <= 0 {
		return args
	}
	if level > maxVerboseLevel {
		level = maxVerboseLevel
	}
	return append(args, "-"+strings.Repeat("v", level))
}

// appendExtraVars appends all extra-vars to the args slice.
func appendExtraVars(args []string, extraVars []string) []string {
	for _, ev := range extraVars {
		if ev != "" {
			args = append(args, "--extra-vars", ev)
		}
	}
	return args
}

// versionCommand creates the command to display the Ansible version.
func (p *Playbook) versionCommand(ctx context.Context) *exec.Cmd {
	return exec.CommandContext(ctx, "ansible", "--version") //nolint:gosec // intentional subprocess execution; args are derived from trusted configuration
}

// buildGalaxyCommand constructs a galaxy command using a base command and given options.
// The context is passed through to exec.CommandContext for cancellation support.
func (p *Playbook) buildGalaxyCommand(ctx context.Context, base []string, opts []argOption) *exec.Cmd {
	args := make([]string, len(base))
	copy(args, base)
	for _, opt := range opts {
		args = applyOption(args, opt)
	}
	args = addVerbose(args, p.Config.Verbose)
	return exec.CommandContext(ctx, "ansible-galaxy", args...) //nolint:gosec // intentional subprocess execution; args are derived from trusted configuration
}

// commonGalaxyOptions returns a fresh slice of options shared by both role and collection install commands.
func (p *Playbook) commonGalaxyOptions() []argOption {
	return []argOption{
		{flag: "--server", value: p.Config.GalaxyAPIServerURL},
		{flag: "--ignore-certs", value: p.Config.GalaxyIgnoreCerts},
		{flag: "--timeout", value: p.Config.GalaxyTimeout},
		{flag: "--force", value: p.Config.GalaxyForce},
		{flag: "--force-with-deps", value: p.Config.GalaxyForceWithDeps},
	}
}

// galaxyRoleCommand creates the command to install Ansible Galaxy roles.
func (p *Playbook) galaxyRoleCommand(ctx context.Context) *exec.Cmd {
	opts := append(p.commonGalaxyOptions(),
		argOption{flag: "--role-file", value: p.Config.galaxyRequirementsFile()},
		argOption{flag: "--roles-path", value: p.Config.GalaxyRolesPath},
		argOption{flag: "--no-deps", value: p.Config.GalaxyNoDeps},
	)
	return p.buildGalaxyCommand(ctx, []string{"role", "install"}, opts)
}

// galaxyCollectionCommand creates the command to install Ansible Galaxy collections.
func (p *Playbook) galaxyCollectionCommand(ctx context.Context) *exec.Cmd {
	opts := append(p.commonGalaxyOptions(),
		argOption{flag: "--requirements-file", value: p.Config.galaxyRequirementsFile()},
		argOption{flag: "--collections-path", value: p.Config.GalaxyCollectionsPath},
		argOption{flag: "--pre", value: p.Config.GalaxyPre},
		argOption{flag: "--upgrade", value: p.Config.GalaxyUpgrade},
		argOption{flag: "--keyring", value: p.Config.GalaxyKeyring},
		argOption{flag: "--disable-gpg-verify", value: p.Config.GalaxyDisableGPGVerify},
		argOption{flag: "--required-valid-signature-count", value: p.Config.GalaxyRequiredValidSignatureCount},
		argOption{flag: "--ignore-signature-status-code", value: p.Config.GalaxyIgnoreSignatureStatusCodes},
		argOption{flag: "--signature", value: p.Config.GalaxySignature},
		argOption{flag: "--offline", value: p.Config.GalaxyOffline},
	)
	return p.buildGalaxyCommand(ctx, []string{"collection", "install"}, opts)
}

// ansibleCommand creates the command to run an Ansible playbook with the specified inventories.
func (p *Playbook) ansibleCommand(ctx context.Context, inventories []string) *exec.Cmd {
	var args []string
	for _, inv := range inventories {
		args = append(args, "--inventory", inv)
	}
	// The inspection flags combine with each other and with all other options
	// (--limit, --tags, vault, ...), which affect what they report.
	options := []argOption{
		{flag: "--syntax-check", value: p.Config.SyntaxCheck},
		{flag: "--list-hosts", value: p.Config.ListHosts},
		{flag: "--list-tags", value: p.Config.ListTags},
		{flag: "--list-tasks", value: p.Config.ListTasks},
		{flag: "--check", value: p.Config.Check},
		{flag: "--diff", value: p.Config.Diff},
		{flag: "--flush-cache", value: p.Config.FlushCache},
		{flag: "--force-handlers", value: p.Config.ForceHandlers},
		{flag: "--step", value: p.Config.Step},
		{flag: "--forks", value: p.Config.Forks},
		{flag: "--user", value: p.Config.User},
		{flag: "--connection", value: p.Config.Connection},
		{flag: "--timeout", value: p.Config.Timeout},
		{flag: "--limit", value: p.Config.Limit},
		{flag: "--ssh-common-args", value: p.Config.SSHCommonArgs},
		{flag: "--sftp-extra-args", value: p.Config.SFTPExtraArgs},
		{flag: "--scp-extra-args", value: p.Config.SCPExtraArgs},
		{flag: "--ssh-extra-args", value: p.Config.SSHExtraArgs},
		{flag: "--ask-become-pass", value: p.Config.AskBecomePass},
		{flag: "--ask-pass", value: p.Config.AskPass},
		{flag: "--ask-vault-pass", value: p.Config.AskVaultPass},
		{flag: "--become", value: p.Config.Become},
		{flag: "--become-method", value: p.Config.BecomeMethod},
		{flag: "--become-user", value: p.Config.BecomeUser},
		{flag: "--private-key", value: p.Config.PrivateKeyFile},
		{flag: "--vault-id", value: p.Config.VaultID},
		{flag: "--vault-password-file", value: p.Config.VaultPasswordFile},
		{flag: "--tags", value: p.Config.Tags},
		{flag: "--skip-tags", value: p.Config.SkipTags},
		{flag: "--start-at-task", value: p.Config.StartAtTask},
		{flag: "--module-path", value: p.Config.ModulePath},
	}
	for _, opt := range options {
		args = applyOption(args, opt)
	}

	args = appendExtraVars(args, p.Config.ExtraVars)
	args = addVerbose(args, p.Config.Verbose)
	args = append(args, p.Config.Playbooks...)
	return exec.CommandContext(ctx, "ansible-playbook", args...) //nolint:gosec // intentional subprocess execution; args are derived from trusted configuration
}

// sensitiveFlags lists command-line flags whose values should be masked in trace output.
var sensitiveFlags = map[string]bool{
	"--extra-vars":          true,
	"-e":                    true,
	"--vault-password-file": true,
	"--vault-pass-file":     true,
	"--private-key":         true,
	"--api-key":             true,
}

// trace prints the full command line to the configured TraceOutput (or os.Stdout).
// Values of sensitive flags (e.g. --extra-vars, --vault-password-file) are masked.
func (p *Playbook) trace(cmd *exec.Cmd) {
	w := p.TraceOutput
	if w == nil {
		w = os.Stdout
	}
	_, _ = fmt.Fprintf(w, "$ %s\n", strings.Join(maskSensitiveArgs(cmd.Args), " "))
}

// maskSensitiveArgs returns a copy of args with values of sensitive flags replaced by "******".
func maskSensitiveArgs(args []string) []string {
	masked := make([]string, len(args))
	copy(masked, args)
	for i := 0; i < len(masked); i++ {
		arg := masked[i]
		// Handle --flag=value form (e.g. --extra-vars=secret)
		for flag := range sensitiveFlags {
			if strings.HasPrefix(arg, flag+"=") {
				masked[i] = flag + "=******"
				break
			}
		}
		// Handle --flag value form (e.g. --extra-vars secret)
		if sensitiveFlags[arg] && i < len(masked)-1 {
			masked[i+1] = "******"
			i++ // skip the value we just masked
		}
	}
	return masked
}

// isValidSSHKey validates that the content is in proper PEM format for SSH keys.
// It checks for matching BEGIN and END markers in the correct order to ensure valid PEM structure.
func isValidSSHKey(content string) bool {
	// List of valid private key types
	keyTypes := []string{
		"PRIVATE KEY",
		"ENCRYPTED PRIVATE KEY",
		"RSA PRIVATE KEY",
		"OPENSSH PRIVATE KEY",
		"EC PRIVATE KEY",
		"DSA PRIVATE KEY",
	}

	// Check for matching BEGIN and END markers for the same key type, in the correct order
	for _, keyType := range keyTypes {
		beginMarker := "-----BEGIN " + keyType + "-----"
		endMarker := "-----END " + keyType + "-----"

		beginIdx := strings.Index(content, beginMarker)
		if beginIdx == -1 {
			continue
		}

		// Search for END marker after BEGIN marker
		endIdx := strings.Index(content[beginIdx+len(beginMarker):], endMarker)
		if endIdx != -1 {
			return true
		}
	}

	return false
}

// writeTempFile creates a temporary file in the specified directory with the given prefix,
// writes the content to the file, closes it and sets the specified permissions.
// This function normalizes line endings (CRLF → LF) and ensures a trailing newline for SSH keys.
// For SSH keys, it also validates the PEM format to catch potential issues early.
func writeTempFile(tempDir, prefix, content string, perm os.FileMode) (string, error) {
	// Normalize line endings (CRLF → LF) to ensure compatibility across platforms
	content = strings.ReplaceAll(content, "\r\n", "\n")

	// Ensure trailing newline for SSH keys and other sensitive files
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	// Validate SSH key format if this is a key file
	// Only validate files with explicit key-related prefixes to avoid false positives
	if strings.HasPrefix(prefix, "ansible-key-") || strings.HasPrefix(prefix, "ssh-key-") {
		if !isValidSSHKey(content) {
			return "", errors.New("invalid SSH key format: must be in PEM format with proper BEGIN/END markers")
		}
	}

	tmpFile, err := os.CreateTemp(tempDir, prefix)
	if err != nil {
		return "", fmt.Errorf("could not create temp file: %w", err)
	}
	filename := tmpFile.Name()

	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(filename)
		return "", fmt.Errorf("could not write temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(filename)
		return "", fmt.Errorf("could not close temp file: %w", err)
	}

	if err := chmodFile(filename, perm); err != nil {
		_ = os.Remove(filename)
		return "", fmt.Errorf("could not set permissions on temp file: %w", err)
	}

	return filename, nil
}

// chmodFile is os.Chmod indirected through a package var so the permission-set
// failure path of writeTempFile can be exercised deterministically in tests.
// Production always uses os.Chmod.
var chmodFile = os.Chmod
