package ansible

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type Config struct {
	Become                            bool
	BecomeMethod                      string
	BecomeUser                        string
	Check                             bool
	Connection                        string
	Diff                              bool
	ExtraVars                         []string
	FlushCache                        bool
	ForceHandlers                     bool
	Forks                             int
	GalaxyAPIKey                      string
	GalaxyAPIServerURL                string
	GalaxyCollectionsPath             string
	GalaxyDisableGPGVerify            bool
	GalaxyFile                        string
	GalaxyForce                       bool
	GalaxyForceWithDeps               bool
	GalaxyIgnoreCerts                 bool
	GalaxyIgnoreSignatureStatusCodes  []string
	GalaxyKeyring                     string
	GalaxyOffline                     bool
	GalaxyPre                         bool
	GalaxyRequiredValidSignatureCount int
	GalaxyRequirementsFile            string
	GalaxySignature                   string
	GalaxyTimeout                     int
	GalaxyUpgrade                     bool
	GalaxyNoDeps                      bool
	Inventories                       []string
	Limit                             string
	ListHosts                         bool
	ListTags                          bool
	ListTasks                         bool
	ModulePath                        []string
	Playbooks                         []string
	PrivateKey                        string
	PrivateKeyFile                    string
	Requirements                      string
	SCPExtraArgs                      string
	SFTPExtraArgs                     string
	SkipTags                          string
	SSHCommonArgs                     string
	SSHExtraArgs                      string
	StartAtTask                       string
	SyntaxCheck                       bool
	Tags                              string
	Timeout                           int
	User                              string
	VaultID                           string
	VaultPassword                     string
	VaultPasswordFile                 string
	Verbose                           int
}

type AnsiblePlaybook struct {
	Config Config
}

func (p *AnsiblePlaybook) Exec() error {
	if err := p.playbooks(); err != nil {
		return err
	}

	if p.Config.PrivateKey != "" {
		if err := p.privateKey(); err != nil {
			return err
		}

		defer os.Remove(p.Config.PrivateKeyFile)
	}

	if p.Config.VaultPassword != "" {
		if err := p.vaultPass(); err != nil {
			return err
		}

		defer os.Remove(p.Config.VaultPasswordFile)
	}

	commands := []*exec.Cmd{
		p.versionCommand(),
	}

	if p.Config.GalaxyFile != "" {
		commands = append(commands, p.galaxyRoleCommand())
		commands = append(commands, p.galaxyCollectionCommand())
	}

	for _, inventory := range p.Config.Inventories {
		commands = append(commands, p.ansibleCommand(inventory))
	}

	for _, cmd := range commands {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "ANSIBLE_FORCE_COLOR=1")
		cmd.Env = append(cmd.Env, "ANSIBLE_GALAXY_DISPLAY_PROGRESS=0")

		trace(cmd)

		if err := cmd.Run(); err != nil {
			return err
		}
	}

	return nil
}

func (p *AnsiblePlaybook) privateKey() error {
	tmpfile, err := os.CreateTemp("", "privateKey")
	if err != nil {
		return errors.Wrap(err, "failed to create private key file")
	}

	if _, err := tmpfile.Write([]byte(p.Config.PrivateKey)); err != nil {
		return errors.Wrap(err, "failed to write private key file")
	}

	if err := tmpfile.Close(); err != nil {
		return errors.Wrap(err, "failed to close private key file")
	}

	p.Config.PrivateKeyFile = tmpfile.Name()
	return nil
}

func (p *AnsiblePlaybook) vaultPass() error {
	tmpfile, err := os.CreateTemp("", "vaultPass")
	if err != nil {
		return errors.Wrap(err, "failed to create vault password file")
	}

	if _, err := tmpfile.Write([]byte(p.Config.VaultPassword)); err != nil {
		return errors.Wrap(err, "failed to write vault password file")
	}

	if err := tmpfile.Close(); err != nil {
		return errors.Wrap(err, "failed to close vault password file")
	}

	p.Config.VaultPasswordFile = tmpfile.Name()
	return nil
}

func (p *AnsiblePlaybook) playbooks() error {
	var (
		playbooks []string
	)

	for _, p := range p.Config.Playbooks {
		files, err := filepath.Glob(p)

		if err != nil {
			playbooks = append(playbooks, p)
			continue
		}

		playbooks = append(playbooks, files...)
	}

	if len(playbooks) == 0 {
		return errors.New("failed to find playbook files")
	}

	p.Config.Playbooks = playbooks
	return nil
}

func (p *AnsiblePlaybook) versionCommand() *exec.Cmd {
	args := []string{
		"--version",
	}

	return exec.Command(
		"ansible",
		args...,
	)
}

func (p *AnsiblePlaybook) galaxyRoleCommand() *exec.Cmd {
	args := []string{
		"role",
		"install",
		"--role-file",
		p.Config.GalaxyFile,
	}

	if p.Config.GalaxyAPIServerURL != "" {
		args = append(args, "--server", p.Config.GalaxyAPIServerURL)
	}

	if p.Config.GalaxyAPIKey != "" {
		args = append(args, "--api-key", p.Config.GalaxyAPIKey)
	}

	if p.Config.GalaxyIgnoreCerts {
		args = append(args, "--ignore-certs")
	}

	if p.Config.GalaxyTimeout != 0 {
		args = append(args, "--timeout", strconv.Itoa(p.Config.GalaxyTimeout))
	}

	if p.Config.GalaxyForce {
		args = append(args, "--force")
	}

	if p.Config.GalaxyNoDeps {
		args = append(args, "--no-deps")
	}

	if p.Config.GalaxyForceWithDeps {
		args = append(args, "--force-with-deps")
	}

	if p.Config.Verbose > 0 {
		args = append(args, fmt.Sprintf("-%s", strings.Repeat("v", p.Config.Verbose)))
	}

	return exec.Command(
		"ansible-galaxy",
		args...,
	)
}

func (p *AnsiblePlaybook) galaxyCollectionCommand() *exec.Cmd {
	args := []string{
		"collection",
		"install",
		"--requirements-file",
		p.Config.GalaxyFile,
	}

	if p.Config.GalaxyAPIServerURL != "" {
		args = append(args, "--server", p.Config.GalaxyAPIServerURL)
	}

	if p.Config.GalaxyAPIKey != "" {
		args = append(args, "--api-key", p.Config.GalaxyAPIKey)
	}

	if p.Config.GalaxyIgnoreCerts {
		args = append(args, "--ignore-certs")
	}

	if p.Config.GalaxyTimeout != 0 {
		args = append(args, "--timeout", strconv.Itoa(p.Config.GalaxyTimeout))
	}

	if p.Config.GalaxyForceWithDeps {
		args = append(args, "--force-with-deps")
	}

	if p.Config.GalaxyCollectionsPath != "" {
		args = append(args, "--collections-path", p.Config.GalaxyCollectionsPath)
	}

	if p.Config.GalaxyRequirementsFile != "" {
		args = append(args, "--requirements-file", p.Config.GalaxyRequirementsFile)
	}

	if p.Config.GalaxyPre {
		args = append(args, "--pre")
	}

	if p.Config.GalaxyUpgrade {
		args = append(args, "--upgrade")
	}

	if p.Config.GalaxyForce {
		args = append(args, "--force")
	}

	if p.Config.Verbose > 0 {
		verboseFlag := fmt.Sprintf("-%s", strings.Repeat("v", p.Config.Verbose))
		args = append(args, verboseFlag)
	}

	return exec.Command(
		"ansible-galaxy",
		args...,
	)
}

func (p *AnsiblePlaybook) ansibleCommand(inventory string) *exec.Cmd {
	args := []string{
		"--inventory",
		inventory,
	}

	if p.Config.SyntaxCheck {
		args = append(args, "--syntax-check")
		args = append(args, p.Config.Playbooks...)

		return exec.Command(
			"ansible-playbook",
			args...,
		)
	}

	if p.Config.ListHosts {
		args = append(args, "--list-hosts")
		args = append(args, p.Config.Playbooks...)

		return exec.Command(
			"ansible-playbook",
			args...,
		)
	}

	for _, v := range p.Config.ExtraVars {
		args = append(args, "--extra-vars", v)
	}

	if p.Config.Check {
		args = append(args, "--check")
	}

	if p.Config.Diff {
		args = append(args, "--diff")
	}

	if p.Config.FlushCache {
		args = append(args, "--flush-cache")
	}

	if p.Config.ForceHandlers {
		args = append(args, "--force-handlers")
	}

	if p.Config.Forks != 5 {
		args = append(args, "--forks", strconv.Itoa(p.Config.Forks))
	}

	if p.Config.Limit != "" {
		args = append(args, "--limit", p.Config.Limit)
	}

	if p.Config.ListTags {
		args = append(args, "--list-tags")
	}

	if p.Config.ListTasks {
		args = append(args, "--list-tasks")
	}

	if len(p.Config.ModulePath) > 0 {
		args = append(args, "--module-path", strings.Join(p.Config.ModulePath, ":"))
	}

	if p.Config.SkipTags != "" {
		args = append(args, "--skip-tags", p.Config.SkipTags)
	}

	if p.Config.StartAtTask != "" {
		args = append(args, "--start-at-task", p.Config.StartAtTask)
	}

	if p.Config.Tags != "" {
		args = append(args, "--tags", p.Config.Tags)
	}

	if p.Config.VaultID != "" {
		args = append(args, "--vault-id", p.Config.VaultID)
	}

	if p.Config.VaultPasswordFile != "" {
		args = append(args, "--vault-password-file", p.Config.VaultPasswordFile)
	}

	if p.Config.PrivateKeyFile != "" {
		args = append(args, "--private-key", p.Config.PrivateKeyFile)
	}

	if p.Config.User != "" {
		args = append(args, "--user", p.Config.User)
	}

	if p.Config.Connection != "" {
		args = append(args, "--connection", p.Config.Connection)
	}

	if p.Config.Timeout != 0 {
		args = append(args, "--timeout", strconv.Itoa(p.Config.Timeout))
	}

	if p.Config.SSHCommonArgs != "" {
		args = append(args, "--ssh-common-args", p.Config.SSHCommonArgs)
	}

	if p.Config.SFTPExtraArgs != "" {
		args = append(args, "--sftp-extra-args", p.Config.SFTPExtraArgs)
	}

	if p.Config.SCPExtraArgs != "" {
		args = append(args, "--scp-extra-args", p.Config.SCPExtraArgs)
	}

	if p.Config.SSHExtraArgs != "" {
		args = append(args, "--ssh-extra-args", p.Config.SSHExtraArgs)
	}

	if p.Config.Become {
		args = append(args, "--become")
	}

	if p.Config.BecomeMethod != "" {
		args = append(args, "--become-method", p.Config.BecomeMethod)
	}

	if p.Config.BecomeUser != "" {
		args = append(args, "--become-user", p.Config.BecomeUser)
	}

	if p.Config.Verbose > 0 {
		verboseFlag := fmt.Sprintf("-%s", strings.Repeat("v", p.Config.Verbose))
		args = append(args, verboseFlag)
	}

	args = append(args, p.Config.Playbooks...)

	return exec.Command(
		"ansible-playbook",
		args...,
	)
}

func trace(cmd *exec.Cmd) {
	fmt.Println("$", strings.Join(cmd.Args, " "))
}
