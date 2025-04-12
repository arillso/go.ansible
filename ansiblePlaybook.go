// Package ansible provides functionality to run Ansible playbooks with configurable options.
// It supports building command line parameters, handling temporary files, and context-based execution.
package ansible

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Config contains configuration options for running Ansible playbooks.
type Config struct {
	// General options
	Become                                 bool
	BecomeMethod, BecomeUser               string
	User                                   string
	PrivateKey, PrivateKeyFile             string
	AskBecomePass, AskPass                 bool
	Check, Diff, FlushCache, ForceHandlers bool
	SyntaxCheck                            bool
	ListHosts, ListTags, ListTasks         bool
	Step                                   bool
	Connection                             string
	Timeout, Forks                         int

	// SSH options
	SSHCommonArgs, SSHExtraArgs string
	SCPExtraArgs, SFTPExtraArgs string
	SSHTransferMethod           string

	// Playbook options
	Inventories                 []string
	Playbooks                   []string
	Limit                       string
	ExtraVars                   []string
	StartAtTask, Tags, SkipTags string
	ModulePath                  []string
	ModuleName                  string
	Verbose                     int
	NoColor                     bool

	// Vault options
	VaultID, VaultPassword, VaultPasswordFile string
	AskVaultPass                              bool

	// Facts options
	FactPath           string
	InvalidateCache    bool
	FactCaching        string
	FactCachingTimeout int

	// Galaxy options
	GalaxyFile                        string
	GalaxyAPIKey, GalaxyAPIServerURL  string
	GalaxyCollectionsPath             string
	GalaxyDisableGPGVerify            bool
	GalaxyForce, GalaxyForceWithDeps  bool
	GalaxyNoDeps, GalaxyIgnoreCerts   bool
	GalaxyIgnoreSignatureStatusCodes  []string
	GalaxyKeyring                     string
	GalaxyOffline, GalaxyPre          bool
	GalaxyRequiredValidSignatureCount int
	GalaxyRequirementsFile            string
	GalaxySignature                   string
	GalaxyTimeout                     int
	GalaxyUpgrade                     bool

	// Other options
	CallbackWhitelist string
	PollInterval      int
	GatherSubset      string
	GatherTimeout     int
	StrategyPlugin    string
	MaxFailPercentage int
	AnyErrorsFatal    bool
	Requirements      string
	ModuleDefaults    map[string]string
	ConfigFile        string
	MetadataExport    string

	// Optional: directory for temporary files
	TempDir string
}

// Playbook represents an execution of an Ansible playbook run.
type Playbook struct {
	Config    Config
	Debug     bool // Enables additional logging output
	tempFiles []string
}

// NewPlaybook returns a new instance of Playbook with default values.
func NewPlaybook() *Playbook {
	return &Playbook{
		Config: Config{
			Forks:   5,
			TempDir: os.TempDir(),
		},
	}
}

// Exec runs the configured Ansible playbooks using the provided context.
// It resolves playbook paths, prepares temporary files, builds and executes commands,
// and cleans up temporary files afterward.
func (p *Playbook) Exec(ctx context.Context) error {
	defer p.cleanupTempFiles()

	if err := p.resolvePlaybooks(); err != nil {
		return errors.Wrap(err, "failed to resolve playbooks")
	}

	if err := p.prepareTempFiles(); err != nil {
		return errors.Wrap(err, "failed to prepare temporary files")
	}

	cmds, err := p.buildCommands(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to build commands")
	}

	return p.runCommands(ctx, cmds)
}

// resolvePlaybooks resolves playbook patterns into concrete file paths and validates their existence.
func (p *Playbook) resolvePlaybooks() error {
	if len(p.Config.Playbooks) == 0 {
		return errors.New("no playbooks specified")
	}

	var playbooks []string
	for _, pattern := range p.Config.Playbooks {
		if files, err := filepath.Glob(pattern); err == nil && len(files) > 0 {
			for _, file := range files {
				if _, err := os.Stat(file); err == nil {
					playbooks = append(playbooks, file)
				} else {
					return errors.Wrapf(err, "playbook not found: %s", file)
				}
			}
		} else if _, err := os.Stat(pattern); err == nil {
			playbooks = append(playbooks, pattern)
		} else {
			return errors.Errorf("playbook not found: %s", pattern)
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
			return errors.Wrap(err, "could not create private key file")
		}
		p.Config.PrivateKeyFile = file
		p.tempFiles = append(p.tempFiles, file)
	}
	if p.Config.VaultPassword != "" {
		file, err := writeTempFile(p.Config.TempDir, "ansible-vault-", p.Config.VaultPassword, 0600)
		if err != nil {
			return errors.Wrap(err, "could not create vault password file")
		}
		p.Config.VaultPasswordFile = file
		p.tempFiles = append(p.tempFiles, file)
	}
	return nil
}

// cleanupTempFiles removes all temporary files created during execution.
func (p *Playbook) cleanupTempFiles() {
	for _, f := range p.tempFiles {
		os.Remove(f)
	}
}

// buildCommands constructs the list of exec.Cmd commands (version, galaxy, playbook)
// based on the configuration and given context.
func (p *Playbook) buildCommands(ctx context.Context) ([]*exec.Cmd, error) {
	var cmds []*exec.Cmd

	// Version command
	cmds = append(cmds, p.versionCommand(ctx))

	// Galaxy commands (if GalaxyFile is set)
	if p.Config.GalaxyFile != "" {
		if _, err := os.Stat(p.Config.GalaxyFile); os.IsNotExist(err) {
			return nil, errors.Errorf("galaxy file not found: %s", p.Config.GalaxyFile)
		}
		cmds = append(cmds, p.galaxyRoleCommand(ctx), p.galaxyCollectionCommand(ctx))
	}

	// Build Ansible commands for each inventory
	for _, inv := range p.Config.Inventories {
		if err := validateInventory(inv); err != nil {
			return nil, err
		}
		cmds = append(cmds, p.ansibleCommand(ctx, inv))
	}

	return cmds, nil
}

// runCommands executes the given commands sequentially, using the provided context.
func (p *Playbook) runCommands(ctx context.Context, cmds []*exec.Cmd) error {
	_ = ctx

	envVars := buildCustomEnvVars(p.Config)
	for i, cmd := range cmds {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), "ANSIBLE_FORCE_COLOR=1", "ANSIBLE_GALAXY_DISPLAY_PROGRESS=0")
		cmd.Env = append(cmd.Env, envVars...)
		if p.Debug {
			p.trace(cmd)
		}
		if err := cmd.Run(); err != nil {
			cmdName := filepath.Base(cmd.Path)
			return errors.Wrapf(err, "error executing %s (command %d/%d)", cmdName, i+1, len(cmds))
		}
	}
	return nil
}

// validateInventory checks whether the inventory file exists.
// For inline inventories (containing a comma), it is assumed to be valid.
func validateInventory(inv string) error {
	if strings.Contains(inv, ",") {
		return nil
	}
	if _, err := os.Stat(inv); os.IsNotExist(err) {
		return errors.Errorf("inventory not found: %s", inv)
	}
	return nil
}

// buildCustomEnvVars constructs additional environment variables for Ansible.
func buildCustomEnvVars(cfg Config) []string {
	var env []string
	if cfg.ConfigFile != "" {
		if _, err := os.Stat(cfg.ConfigFile); err == nil {
			env = append(env, "ANSIBLE_CONFIG="+cfg.ConfigFile)
		}
	}
	if cfg.FactCaching != "" {
		env = append(env, "ANSIBLE_FACT_CACHING="+cfg.FactCaching)
	}
	if cfg.FactCachingTimeout > 0 {
		env = append(env, "ANSIBLE_FACT_CACHING_TIMEOUT="+strconv.Itoa(cfg.FactCachingTimeout))
	}
	return env
}

// argOption represents a single command-line flag option.
type argOption struct {
	flag  string
	value interface{}
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
	verboseFlag := "-"
	for i := 0; i < level && i < 4; i++ {
		verboseFlag += "v"
	}
	return append(args, verboseFlag)
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
	return exec.CommandContext(ctx, "ansible", "--version")
}

// buildGalaxyCommand constructs a galaxy command using a base command and given options.
func (p *Playbook) buildGalaxyCommand(ctx context.Context, base []string, opts []argOption) *exec.Cmd {
	// Ensure ctx is used.
	_ = ctx
	args := make([]string, len(base))
	copy(args, base)
	for _, opt := range opts {
		args = applyOption(args, opt)
	}
	args = addVerbose(args, p.Config.Verbose)
	return exec.CommandContext(ctx, "ansible-galaxy", args...)
}

// jscpd:ignore-start
// galaxyRoleCommand creates the command to install Ansible Galaxy roles.
func (p *Playbook) galaxyRoleCommand(ctx context.Context) *exec.Cmd {
	// Mark ctx as used explicitly
	_ = ctx
	opts := []argOption{
		{flag: "--role-file", value: p.Config.GalaxyFile},
		{flag: "--server", value: p.Config.GalaxyAPIServerURL},
		{flag: "--api-key", value: p.Config.GalaxyAPIKey},
		{flag: "--ignore-certs", value: p.Config.GalaxyIgnoreCerts},
		{flag: "--timeout", value: p.Config.GalaxyTimeout},
		{flag: "--force", value: p.Config.GalaxyForce},
		{flag: "--no-deps", value: p.Config.GalaxyNoDeps},
		{flag: "--force-with-deps", value: p.Config.GalaxyForceWithDeps},
	}
	return p.buildGalaxyCommand(ctx, []string{"role", "install"}, opts)
}

// galaxyCollectionCommand creates the command to install Ansible Galaxy collections.
func (p *Playbook) galaxyCollectionCommand(ctx context.Context) *exec.Cmd {
	// Mark ctx as used explicitly
	_ = ctx
	opts := []argOption{
		{flag: "--requirements-file", value: p.Config.GalaxyFile},
		{flag: "--server", value: p.Config.GalaxyAPIServerURL},
		{flag: "--api-key", value: p.Config.GalaxyAPIKey},
		{flag: "--ignore-certs", value: p.Config.GalaxyIgnoreCerts},
		{flag: "--timeout", value: p.Config.GalaxyTimeout},
		{flag: "--force-with-deps", value: p.Config.GalaxyForceWithDeps},
		{flag: "--collections-path", value: p.Config.GalaxyCollectionsPath},
		{flag: "--pre", value: p.Config.GalaxyPre},
		{flag: "--upgrade", value: p.Config.GalaxyUpgrade},
		{flag: "--force", value: p.Config.GalaxyForce},
	}
	return p.buildGalaxyCommand(ctx, []string{"collection", "install"}, opts)
}

// jscpd:ignore-end

// ansibleCommand creates the command to run an Ansible playbook for the specified inventory.
func (p *Playbook) ansibleCommand(ctx context.Context, inventory string) *exec.Cmd {
	args := []string{"--inventory", inventory}
	if p.Config.SyntaxCheck || p.Config.ListHosts {
		flag := "--syntax-check"
		if p.Config.ListHosts {
			flag = "--list-hosts"
		}
		args = append(args, flag)
		args = append(args, p.Config.Playbooks...)
		return exec.CommandContext(ctx, "ansible-playbook", args...)
	}

	options := []argOption{
		{flag: "--check", value: p.Config.Check},
		{flag: "--diff", value: p.Config.Diff},
		{flag: "--flush-cache", value: p.Config.FlushCache},
		{flag: "--force-handlers", value: p.Config.ForceHandlers},
		{flag: "--step", value: p.Config.Step},
		{flag: "--no-color", value: p.Config.NoColor},
		{flag: "--forks", value: p.Config.Forks},
		{flag: "--user", value: p.Config.User},
		{flag: "--connection", value: p.Config.Connection},
		{flag: "--timeout", value: p.Config.Timeout},
		{flag: "--limit", value: p.Config.Limit},
		{flag: "--ssh-common-args", value: p.Config.SSHCommonArgs},
		{flag: "--sftp-extra-args", value: p.Config.SFTPExtraArgs},
		{flag: "--scp-extra-args", value: p.Config.SCPExtraArgs},
		{flag: "--ssh-extra-args", value: p.Config.SSHExtraArgs},
		{flag: "--ssh-transfer-method", value: p.Config.SSHTransferMethod},
		{flag: "--ask-become-pass", value: p.Config.AskBecomePass},
		{flag: "--ask-pass", value: p.Config.AskPass},
		{flag: "--ask-vault-pass", value: p.Config.AskVaultPass},
		{flag: "--become", value: p.Config.Become},
		{flag: "--become-method", value: p.Config.BecomeMethod},
		{flag: "--become-user", value: p.Config.BecomeUser},
		{flag: "--private-key", value: p.Config.PrivateKeyFile},
		{flag: "--vault-id", value: p.Config.VaultID},
		{flag: "--vault-password-file", value: p.Config.VaultPasswordFile},
		{flag: "--callback-whitelist", value: p.Config.CallbackWhitelist},
		{flag: "--poll-interval", value: p.Config.PollInterval},
		{flag: "--strategy", value: p.Config.StrategyPlugin},
		{flag: "--max-fail-percentage", value: p.Config.MaxFailPercentage},
		{flag: "--any-errors-fatal", value: p.Config.AnyErrorsFatal},
		{flag: "--gather-subset", value: p.Config.GatherSubset},
		{flag: "--gather-timeout", value: p.Config.GatherTimeout},
		{flag: "--tags", value: p.Config.Tags},
		{flag: "--skip-tags", value: p.Config.SkipTags},
		{flag: "--start-at-task", value: p.Config.StartAtTask},
	}
	for _, opt := range options {
		args = applyOption(args, opt)
	}

	args = appendExtraVars(args, p.Config.ExtraVars)
	args = addVerbose(args, p.Config.Verbose)
	args = append(args, p.Config.Playbooks...)
	return exec.CommandContext(ctx, "ansible-playbook", args...)
}

// trace prints the full command line to standard output.
func (p *Playbook) trace(cmd *exec.Cmd) {
	fmt.Printf("$ %s\n", strings.Join(cmd.Args, " "))
}

// writeTempFile creates a temporary file in the specified directory with the given prefix,
// writes the content to the file, closes it and sets the specified permissions.
func writeTempFile(tempDir, prefix, content string, perm os.FileMode) (string, error) {
	tmpFile, err := os.CreateTemp(tempDir, prefix)
	if err != nil {
		return "", errors.Wrap(err, "could not create temp file")
	}
	filename := tmpFile.Name()

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		tmpFile.Close()
		os.Remove(filename)
		return "", errors.Wrap(err, "could not write temp file")
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(filename)
		return "", errors.Wrap(err, "could not close temp file")
	}

	if err := os.Chmod(filename, perm); err != nil {
		os.Remove(filename)
		return "", errors.Wrap(err, "could not set permissions on temp file")
	}

	return filename, nil
}
