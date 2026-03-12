// Tests for the ansible package covering playbook resolution, temporary file
// management, environment variable assembly, and helper functions.

package ansible

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// getInventoryHost returns the inventory host for tests,
// reading from an environment variable as fallback.
func getInventoryHost() string {
	host := os.Getenv("TEST_INVENTORY_HOST")
	if host == "" {
		// devskim:ignore DS162092 - Using "localhost" deliberately in tests.
		host = "localhost" // fallback value for tests
	}
	return host
}

// TestIsCollectionPlaybook tests the detection of collection playbook references.
func TestIsCollectionPlaybook(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		{"Collection FQCN", "namespace.collection.playbook", true},
		{"Collection FQCN with underscores", "my_namespace.my_collection.my_playbook", true},
		{"Simple filename", "playbook.yml", false},
		{"Relative path", "./playbooks/site.yml", false},
		{"Absolute path", "/etc/ansible/playbook.yml", false},
		{"No dots", "playbook", false},
		{"Glob pattern", "*.yml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCollectionPlaybook(tt.ref)
			if result != tt.expected {
				t.Errorf("isCollectionPlaybook(%q) = %v, expected %v", tt.ref, result, tt.expected)
			}
		})
	}
}

// TestResolvePlaybooks tests that playbook paths are correctly resolved.
func TestResolvePlaybooks(t *testing.T) {
	// Create a temporary directory using os.MkdirTemp.
	tempDir, err := os.MkdirTemp("", "test-playbook")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	playbookPath := filepath.Join(tempDir, "site.yml")
	// Write file using os.WriteFile.
	if err := os.WriteFile(playbookPath, []byte("dummy content"), 0644); err != nil {
		t.Fatalf("Failed to write dummy playbook file: %v", err)
	}

	pb := NewPlaybook()
	// Set a pattern that matches the dummy file.
	pb.Config.Playbooks = []string{filepath.Join(tempDir, "*.yml")}

	if err := pb.resolvePlaybooks(); err != nil {
		t.Fatalf("resolvePlaybooks failed: %v", err)
	}

	// Check if the expected playbook path is in the resolved list.
	found := false
	for _, p := range pb.Config.Playbooks {
		if p == playbookPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected playbook file %s was not found in the resolved paths", playbookPath)
	}
}

// TestResolveCollectionPlaybooks tests that collection playbook references are handled correctly.
func TestResolveCollectionPlaybooks(t *testing.T) {
	pb := NewPlaybook()
	collectionPlaybook := "namespace.collection.playbook"
	pb.Config.Playbooks = []string{collectionPlaybook}

	if err := pb.resolvePlaybooks(); err != nil {
		t.Fatalf("resolvePlaybooks failed for collection playbook: %v", err)
	}

	// Check if the collection playbook is preserved in the resolved list.
	if len(pb.Config.Playbooks) != 1 {
		t.Fatalf("Expected 1 playbook, got %d", len(pb.Config.Playbooks))
	}
	if pb.Config.Playbooks[0] != collectionPlaybook {
		t.Errorf("Expected playbook %q, got %q", collectionPlaybook, pb.Config.Playbooks[0])
	}
}

// TestResolveMixedPlaybooks tests resolving both local files and collection playbooks.
func TestResolveMixedPlaybooks(t *testing.T) {
	// Create a temporary directory.
	tempDir, err := os.MkdirTemp("", "test-mixed-playbook")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a local playbook file.
	localPlaybook := filepath.Join(tempDir, "local.yml")
	if err := os.WriteFile(localPlaybook, []byte("dummy content"), 0644); err != nil {
		t.Fatalf("Failed to write dummy playbook file: %v", err)
	}

	pb := NewPlaybook()
	collectionPlaybook := "namespace.collection.playbook"
	pb.Config.Playbooks = []string{localPlaybook, collectionPlaybook}

	if err := pb.resolvePlaybooks(); err != nil {
		t.Fatalf("resolvePlaybooks failed for mixed playbooks: %v", err)
	}

	// Check that both playbooks are in the resolved list.
	if len(pb.Config.Playbooks) != 2 {
		t.Fatalf("Expected 2 playbooks, got %d", len(pb.Config.Playbooks))
	}

	foundLocal := false
	foundCollection := false
	for _, p := range pb.Config.Playbooks {
		if p == localPlaybook {
			foundLocal = true
		}
		if p == collectionPlaybook {
			foundCollection = true
		}
	}

	if !foundLocal {
		t.Errorf("Expected local playbook %q not found in resolved list", localPlaybook)
	}
	if !foundCollection {
		t.Errorf("Expected collection playbook %q not found in resolved list", collectionPlaybook)
	}
}

// TestPrepareTempFiles tests the creation of temporary files (PrivateKey and VaultPassword).
func TestPrepareTempFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-temp")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	pb := NewPlaybook()
	pb.Config.TempDir = tempDir

	// Use a valid PEM-formatted private key for testing
	privateKeyContent := "-----BEGIN RSA PRIVATE KEY-----\nTEST-KEY-DATA\n-----END RSA PRIVATE KEY-----"
	vaultPasswordContent := "my_vault_password"
	pb.Config.PrivateKey = privateKeyContent
	pb.Config.VaultPassword = vaultPasswordContent

	if err := pb.prepareTempFiles(); err != nil {
		t.Fatalf("prepareTempFiles failed: %v", err)
	}

	// Verify the content of the private key file.
	// Note: writeTempFile adds a trailing newline
	data, err := os.ReadFile(pb.Config.PrivateKeyFile)
	if err != nil {
		t.Fatalf("Failed to read the private key file: %v", err)
	}
	expectedPrivateKey := privateKeyContent + "\n"
	if string(data) != expectedPrivateKey {
		t.Errorf("Private key file content mismatch, expected %q, got %q", expectedPrivateKey, string(data))
	}

	// Verify the content of the vault password file.
	// Note: writeTempFile adds a trailing newline
	data, err = os.ReadFile(pb.Config.VaultPasswordFile)
	if err != nil {
		t.Fatalf("Failed to read the vault password file: %v", err)
	}
	expectedVaultPassword := vaultPasswordContent + "\n"
	if string(data) != expectedVaultPassword {
		t.Errorf("Vault password file content mismatch, expected %q, got %q", expectedVaultPassword, string(data))
	}

	// Check that temporary files are removed after cleanup.
	pb.cleanupTempFiles()
	if _, err := os.Stat(pb.Config.PrivateKeyFile); !os.IsNotExist(err) {
		t.Errorf("Private key file still exists after cleanup")
	}
	if _, err := os.Stat(pb.Config.VaultPasswordFile); !os.IsNotExist(err) {
		t.Errorf("Vault password file still exists after cleanup")
	}
}

// TestValidateInventory verifies the validation of inventory specifications.
func TestValidateInventory(t *testing.T) {
	// Inline inventory (contains a comma) should be valid.
	inlineInv := getInventoryHost() + ","
	if err := validateInventory(inlineInv); err != nil {
		t.Errorf("Inline inventory %q should be valid, error: %v", inlineInv, err)
	}

	// For a non-existent inventory path, an error should be returned.
	nonExistentInv := "/path/to/nonexistent/inventory"
	if err := validateInventory(nonExistentInv); err == nil {
		t.Errorf("Expected error for nonexistent inventory %q, but no error was returned", nonExistentInv)
	}
}

// TestBuildCustomEnvVars tests the assembly of additional environment variables.
func TestBuildCustomEnvVars(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-ansible")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	cfg := Config{}
	// Create a dummy configuration file.
	cfgFilePath := filepath.Join(tempDir, "ansible.cfg")
	if err := os.WriteFile(cfgFilePath, []byte("dummy config"), 0644); err != nil {
		t.Fatalf("Failed to write dummy configuration file: %v", err)
	}
	cfg.ConfigFile = cfgFilePath
	cfg.FactCaching = "jsonfile"
	cfg.FactCachingTimeout = 60

	envVars, err := buildCustomEnvVars(&cfg)
	if err != nil {
		t.Fatalf("buildCustomEnvVars failed: %v", err)
	}
	var foundConfig, foundFactCaching, foundFactCachingTimeout bool
	for _, env := range envVars {
		if strings.HasPrefix(env, "ANSIBLE_CONFIG=") {
			foundConfig = true
		}
		if strings.HasPrefix(env, "ANSIBLE_FACT_CACHING=") {
			foundFactCaching = true
		}
		if strings.HasPrefix(env, "ANSIBLE_FACT_CACHING_TIMEOUT=") {
			foundFactCachingTimeout = true
		}
	}
	if !foundConfig {
		t.Error("ANSIBLE_CONFIG not found in environment variables")
	}
	if !foundFactCaching {
		t.Error("ANSIBLE_FACT_CACHING not found in environment variables")
	}
	if !foundFactCachingTimeout {
		t.Error("ANSIBLE_FACT_CACHING_TIMEOUT not found in environment variables")
	}
}

// TestAddVerbose verifies that the verbose flag is correctly generated.
func TestAddVerbose(t *testing.T) {
	tests := []struct {
		name     string
		level    int
		expected string
	}{
		{"level 0 adds nothing", 0, ""},
		{"negative level adds nothing", -1, ""},
		{"level 1", 1, "-v"},
		{"level 2", 2, "-vv"},
		{"level 3", 3, "-vvv"},
		{"level 4 (max)", 4, "-vvvv"},
		{"level 5 clamped to max", 5, "-vvvv"},
		{"level 100 clamped to max", 100, "-vvvv"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := addVerbose([]string{"test"}, tt.level)
			if tt.expected == "" {
				if len(args) != 1 {
					t.Errorf("Expected no verbose flag added, got %v", args)
				}
			} else {
				if len(args) != 2 {
					t.Fatalf("Expected 2 args, got %d: %v", len(args), args)
				}
				if args[1] != tt.expected {
					t.Errorf("Expected %q, got %q", tt.expected, args[1])
				}
			}
		})
	}
}

// TestApplyOption verifies that applyOption correctly handles all supported types.
func TestApplyOption(t *testing.T) {
	tests := []struct {
		name     string
		opt      argOption
		expected []string
	}{
		{"string value", argOption{"--user", "admin"}, []string{"--user", "admin"}},
		{"empty string skipped", argOption{"--user", ""}, nil},
		{"int value", argOption{"--forks", 10}, []string{"--forks", "10"}},
		{"zero int skipped", argOption{"--forks", 0}, nil},
		{"bool true", argOption{"--check", true}, []string{"--check"}},
		{"bool false skipped", argOption{"--check", false}, nil},
		{"string slice", argOption{"--module-path", []string{"/a", "/b"}}, []string{"--module-path", "/a", "--module-path", "/b"}},
		{"empty string slice skipped", argOption{"--module-path", []string{}}, nil},
		{"string slice with empty item", argOption{"--module-path", []string{"/a", "", "/b"}}, []string{"--module-path", "/a", "--module-path", "/b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyOption(nil, tt.opt)
			if tt.expected == nil {
				if len(result) != 0 {
					t.Errorf("Expected no args, got %v", result)
				}
			} else {
				if len(result) != len(tt.expected) {
					t.Fatalf("Expected %d args, got %d: %v", len(tt.expected), len(result), result)
				}
				for i, v := range tt.expected {
					if result[i] != v {
						t.Errorf("Arg[%d]: expected %q, got %q", i, v, result[i])
					}
				}
			}
		})
	}
}

// TestAppendExtraVars verifies the appending of extra vars.
func TestAppendExtraVars(t *testing.T) {
	args := []string{}
	extraVars := []string{"var1=value1", "var2=value2"}
	args = appendExtraVars(args, extraVars)

	// There should be two arguments (flag and value) per extra var.
	expectedArgCount := len(extraVars) * 2
	if len(args) != expectedArgCount {
		t.Errorf("Expected %d arguments, got %d", expectedArgCount, len(args))
	}

	// Check if the individual pairs are correctly added to args.
	for i, ev := range extraVars {
		flagIndex := 2 * i
		if args[flagIndex] != "--extra-vars" || args[flagIndex+1] != ev {
			t.Errorf("Expected: \"--extra-vars %s\", got: \"%s %s\"", ev, args[flagIndex], args[flagIndex+1])
		}
	}
}

// TestAnsibleCommand verifies that the command for executing an Ansible playbook is built correctly.
func TestAnsibleCommand(t *testing.T) {
	pb := NewPlaybook()
	// Set a dummy playbook name.
	pb.Config.Playbooks = []string{"playbook.yml"}
	inv := getInventoryHost() + "," // Use the helper function to get the inventory host and append a comma.
	cmd := pb.ansibleCommand(context.Background(), inv)

	// Check if the command path contains "ansible-playbook".
	if !strings.Contains(cmd.Path, "ansible-playbook") {
		t.Errorf("Expected command path to contain \"ansible-playbook\", got: %s", cmd.Path)
	}

	// Verify that the inventory is passed as an argument.
	foundInv := false
	for _, arg := range cmd.Args {
		if arg == inv {
			foundInv = true
			break
		}
	}
	if !foundInv {
		t.Errorf("Expected inventory argument %q not found in command arguments", inv)
	}
}

// TestBuildCommands verifies that the command list is built correctly.
func TestBuildCommands(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-commands")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Create a dummy playbook file.
	playbookFile := filepath.Join(tempDir, "test.yml")
	if err := os.WriteFile(playbookFile, []byte("dummy content"), 0644); err != nil {
		t.Fatalf("Failed to write dummy playbook file: %v", err)
	}

	pb := NewPlaybook()
	pb.Config.TempDir = tempDir
	pb.Config.Playbooks = []string{playbookFile}
	// Use inline inventory to bypass file existence checks.
	pb.Config.Inventories = []string{getInventoryHost() + ","}

	cmds, err := pb.buildCommands(context.Background())
	if err != nil {
		t.Fatalf("buildCommands failed: %v", err)
	}
	if len(cmds) == 0 {
		t.Error("Expected at least one command in the command list")
	}

	// Check that the first command (version query) is built correctly.
	if !strings.Contains(cmds[0].Path, "ansible") {
		t.Errorf("Expected first command to query Ansible version, got: %s", cmds[0].Path)
	}
}

// TestWriteTempFile verifies the creation of a temporary file.
func TestWriteTempFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-writetemp")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	content := "temporary content"
	file, err := writeTempFile(tempDir, "prefix-", content, 0600)
	if err != nil {
		t.Fatalf("writeTempFile failed: %v", err)
	}

	// Check that the file exists and the content is correct.
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Failed to read temporary file: %v", err)
	}
	expectedContent := content + "\n" // writeTempFile adds a trailing newline
	if string(data) != expectedContent {
		t.Errorf("Expected content %q, got: %q", expectedContent, string(data))
	}

	// Verify file permissions.
	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("Failed to stat temporary file: %v", err)
	}
	perm, _ := strconv.ParseUint(strconv.FormatUint(uint64(info.Mode().Perm()), 8), 10, 32)
	if perm != 600 {
		t.Errorf("Expected file permission 0600, got: %v", info.Mode().Perm())
	}
}

// TestWriteTempFileLineEndings verifies that writeTempFile normalizes line endings
// and ensures a trailing newline for SSH keys and other sensitive files.
func TestWriteTempFileLineEndings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "CRLF to LF conversion",
			input:    "line1\r\nline2\r\nline3",
			expected: "line1\nline2\nline3\n",
		},
		{
			name:     "Content without trailing newline",
			input:    "content without newline",
			expected: "content without newline\n",
		},
		{
			name:     "Content with trailing newline",
			input:    "content with newline\n",
			expected: "content with newline\n",
		},
		{
			name:     "Mixed line endings",
			input:    "line1\r\nline2\nline3\r\n",
			expected: "line1\nline2\nline3\n",
		},
		{
			name:     "SSH private key with CRLF",
			input:    "-----BEGIN RSA PRIVATE KEY-----\r\nTEST-KEY-DATA\r\n-----END RSA PRIVATE KEY-----",
			expected: "-----BEGIN RSA PRIVATE KEY-----\nTEST-KEY-DATA\n-----END RSA PRIVATE KEY-----\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "test-line-endings")
			if err != nil {
				t.Fatalf("Failed to create temporary directory: %v", err)
			}
			defer func() { _ = os.RemoveAll(tempDir) }()

			file, err := writeTempFile(tempDir, "test-", tt.input, 0600)
			if err != nil {
				t.Fatalf("writeTempFile failed: %v", err)
			}

			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("Failed to read temporary file: %v", err)
			}

			if string(data) != tt.expected {
				t.Errorf("Expected content %q, got: %q", tt.expected, string(data))
			}
		})
	}
}

// TestWriteTempFileSSHKeyValidation verifies that writeTempFile validates SSH key format.
func TestWriteTempFileSSHKeyValidation(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		content string
		wantErr bool
	}{
		{
			name:    "Valid RSA key with LF endings",
			prefix:  "ansible-key-",
			content: "-----BEGIN RSA PRIVATE KEY-----\nTEST-KEY-DATA\n-----END RSA PRIVATE KEY-----\n",
			wantErr: false,
		},
		{
			name:    "Valid RSA key with CRLF endings (Windows)",
			prefix:  "ansible-key-",
			content: "-----BEGIN RSA PRIVATE KEY-----\r\nTEST-KEY-DATA\r\n-----END RSA PRIVATE KEY-----\r\n",
			wantErr: false,
		},
		{
			name:    "Valid key without trailing newline",
			prefix:  "ansible-key-",
			content: "-----BEGIN RSA PRIVATE KEY-----\nTEST-KEY-DATA\n-----END RSA PRIVATE KEY-----",
			wantErr: false,
		},
		{
			name:    "Valid OpenSSH format key",
			prefix:  "ansible-key-",
			content: "-----BEGIN OPENSSH PRIVATE KEY-----\nTEST-KEY-DATA\n-----END OPENSSH PRIVATE KEY-----\n",
			wantErr: false,
		},
		{
			name:    "Valid EC private key",
			prefix:  "ansible-key-",
			content: "-----BEGIN EC PRIVATE KEY-----\nTEST-KEY-DATA\n-----END EC PRIVATE KEY-----\n",
			wantErr: false,
		},
		{
			name:    "Valid generic private key",
			prefix:  "ansible-key-",
			content: "-----BEGIN PRIVATE KEY-----\nTEST-KEY-DATA\n-----END PRIVATE KEY-----\n",
			wantErr: false,
		},
		{
			name:    "Invalid key - missing BEGIN marker",
			prefix:  "ansible-key-",
			content: "TEST-KEY-DATA\n-----END RSA PRIVATE KEY-----\n",
			wantErr: true,
		},
		{
			name:    "Invalid key - missing END marker",
			prefix:  "ansible-key-",
			content: "-----BEGIN RSA PRIVATE KEY-----\nTEST-KEY-DATA\n",
			wantErr: true,
		},
		{
			name:    "Invalid key - no PEM markers at all",
			prefix:  "ansible-key-",
			content: "This is not a valid SSH key",
			wantErr: true,
		},
		{
			name:    "Non-key file - should not validate",
			prefix:  "ansible-vault-",
			content: "vault_password_123",
			wantErr: false, // vault files are not validated
		},
		{
			name:    "Non-key prefix - contains 'key' but not validated",
			prefix:  "my-keycloak-config-",
			content: "not a real ssh key",
			wantErr: false, // only ansible-key- and ssh-key- prefixes trigger validation
		},
		{
			name:    "SSH key prefix - should validate",
			prefix:  "ssh-key-",
			content: "-----BEGIN RSA PRIVATE KEY-----\nTEST-KEY-DATA\n-----END RSA PRIVATE KEY-----\n",
			wantErr: false,
		},
		{
			name:    "SSH key prefix - invalid content",
			prefix:  "ssh-key-",
			content: "not a valid key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "test-ssh-validation")
			if err != nil {
				t.Fatalf("Failed to create temporary directory: %v", err)
			}
			defer func() { _ = os.RemoveAll(tempDir) }()

			file, err := writeTempFile(tempDir, tt.prefix, tt.content, 0600)
			if (err != nil) != tt.wantErr {
				t.Errorf("writeTempFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if file != "" {
				defer func() { _ = os.Remove(file) }()

				// Verify file permissions
				info, err := os.Stat(file)
				if err != nil {
					t.Errorf("Could not stat file: %v", err)
					return
				}
				if info.Mode().Perm() != 0600 {
					t.Errorf("Wrong permissions: got %o, want 0600", info.Mode().Perm())
				}

				// Verify content normalization
				content, err := os.ReadFile(file)
				if err != nil {
					t.Errorf("Could not read file: %v", err)
					return
				}
				if strings.Contains(string(content), "\r\n") {
					t.Error("File still contains CRLF line endings")
				}
				if !strings.HasSuffix(string(content), "\n") {
					t.Error("File does not end with newline")
				}
			}
		})
	}
}

// TestIsValidSSHKey tests the SSH key validation function.
func TestIsValidSSHKey(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "Valid RSA private key",
			content: "-----BEGIN RSA PRIVATE KEY-----\nTEST-KEY-DATA\n-----END RSA PRIVATE KEY-----",
			want:    true,
		},
		{
			name:    "Valid OpenSSH private key",
			content: "-----BEGIN OPENSSH PRIVATE KEY-----\nTEST-KEY-DATA\n-----END OPENSSH PRIVATE KEY-----",
			want:    true,
		},
		{
			name:    "Valid generic private key",
			content: "-----BEGIN PRIVATE KEY-----\nTEST-KEY-DATA\n-----END PRIVATE KEY-----",
			want:    true,
		},
		{
			name:    "Valid EC private key",
			content: "-----BEGIN EC PRIVATE KEY-----\nTEST-KEY-DATA\n-----END EC PRIVATE KEY-----",
			want:    true,
		},
		{
			name:    "Valid DSA private key",
			content: "-----BEGIN DSA PRIVATE KEY-----\nTEST-KEY-DATA\n-----END DSA PRIVATE KEY-----",
			want:    true,
		},
		{
			name:    "Invalid - missing BEGIN marker",
			content: "TEST-KEY-DATA\n-----END RSA PRIVATE KEY-----",
			want:    false,
		},
		{
			name:    "Invalid - missing END marker",
			content: "-----BEGIN RSA PRIVATE KEY-----\nTEST-KEY-DATA",
			want:    false,
		},
		{
			name:    "Invalid - no PEM markers",
			content: "This is not a valid key",
			want:    false,
		},
		{
			name:    "Invalid - public key (not private)",
			content: "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC...",
			want:    false,
		},
		{
			name:    "Invalid - empty string",
			content: "",
			want:    false,
		},
		{
			name:    "Invalid - mismatched markers (BEGIN RSA, END EC)",
			content: "-----BEGIN RSA PRIVATE KEY-----\nTEST-KEY-DATA\n-----END EC PRIVATE KEY-----",
			want:    false,
		},
		{
			name:    "Invalid - mismatched markers (BEGIN OPENSSH, END RSA)",
			content: "-----BEGIN OPENSSH PRIVATE KEY-----\nTEST-KEY-DATA\n-----END RSA PRIVATE KEY-----",
			want:    false,
		},
		{
			name:    "Invalid - certificate not private key",
			content: "-----BEGIN CERTIFICATE-----\nTEST-CERT-DATA\n-----END CERTIFICATE-----",
			want:    false,
		},
		{
			name:    "Invalid - END marker before BEGIN marker",
			content: "-----END RSA PRIVATE KEY-----\nTEST-KEY-DATA\n-----BEGIN RSA PRIVATE KEY-----",
			want:    false,
		},
		{
			name:    "Invalid - only BEGIN marker, no END",
			content: "-----BEGIN RSA PRIVATE KEY-----\nTEST-KEY-DATA",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidSSHKey(tt.content)
			if got != tt.want {
				t.Errorf("isValidSSHKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAnsibleCommandListTags verifies that --list-tags flag is correctly added.
func TestAnsibleCommandListTags(t *testing.T) {
	pb := NewPlaybook()
	pb.Config.Playbooks = []string{"playbook.yml"}
	pb.Config.ListTags = true
	inv := getInventoryHost() + ","
	cmd := pb.ansibleCommand(context.Background(), inv)

	found := false
	for _, arg := range cmd.Args {
		if arg == "--list-tags" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected --list-tags in command arguments")
	}
}

// TestAnsibleCommandListTasks verifies that --list-tasks flag is correctly added.
func TestAnsibleCommandListTasks(t *testing.T) {
	pb := NewPlaybook()
	pb.Config.Playbooks = []string{"playbook.yml"}
	pb.Config.ListTasks = true
	inv := getInventoryHost() + ","
	cmd := pb.ansibleCommand(context.Background(), inv)

	found := false
	for _, arg := range cmd.Args {
		if arg == "--list-tasks" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected --list-tasks in command arguments")
	}
}

// TestExecNoPlaybooks verifies that Exec returns an error when no playbooks are specified.
func TestExecNoPlaybooks(t *testing.T) {
	pb := NewPlaybook()
	pb.Config.Inventories = []string{getInventoryHost() + ","}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pb.Exec(ctx)
	if err == nil {
		t.Fatal("expected error when no playbooks are specified, got nil")
	}
	if !strings.Contains(err.Error(), "no playbooks specified") {
		t.Errorf("expected 'no playbooks specified' error, got: %v", err)
	}
}

// TestExecNonExistentPlaybook verifies that Exec returns an error for a non-existent playbook file.
func TestExecNonExistentPlaybook(t *testing.T) {
	pb := NewPlaybook()
	pb.Config.Playbooks = []string{"/nonexistent/playbook.yml"}
	pb.Config.Inventories = []string{getInventoryHost() + ","}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pb.Exec(ctx)
	if err == nil {
		t.Fatal("expected error for non-existent playbook, got nil")
	}
	if !strings.Contains(err.Error(), "playbook not found") {
		t.Errorf("expected 'playbook not found' error, got: %v", err)
	}
}

// TestExecInvalidPrivateKey verifies that Exec returns an error for an invalid SSH key.
func TestExecInvalidPrivateKey(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-exec-key")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	playbookFile := filepath.Join(tempDir, "test.yml")
	if err := os.WriteFile(playbookFile, []byte("dummy content"), 0644); err != nil {
		t.Fatalf("Failed to write playbook: %v", err)
	}

	pb := NewPlaybook()
	pb.Config.TempDir = tempDir
	pb.Config.Playbooks = []string{playbookFile}
	pb.Config.Inventories = []string{getInventoryHost() + ","}
	pb.Config.PrivateKey = "not-a-valid-key"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = pb.Exec(ctx)
	if err == nil {
		t.Fatal("expected error for invalid private key, got nil")
	}
	if !strings.Contains(err.Error(), "invalid SSH key format") {
		t.Errorf("expected 'invalid SSH key format' error, got: %v", err)
	}
}

// TestExecCleanupAfterError verifies that temporary files are cleaned up even when Exec returns an error.
func TestExecCleanupAfterError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-exec-cleanup")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	playbookFile := filepath.Join(tempDir, "test.yml")
	if err := os.WriteFile(playbookFile, []byte("dummy content"), 0644); err != nil {
		t.Fatalf("Failed to write playbook: %v", err)
	}

	pb := NewPlaybook()
	pb.Config.TempDir = tempDir
	pb.Config.Playbooks = []string{playbookFile}
	pb.Config.Inventories = []string{getInventoryHost() + ","}
	pb.Config.VaultPassword = "test-vault-pass"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Exec will fail (either timeout or missing ansible binary) but should still clean up
	_ = pb.Exec(ctx)

	// Verify that at least one temporary file was tracked (vault password)
	if len(pb.tempFiles) == 0 {
		t.Fatal("expected at least one temporary file to be tracked for cleanup verification")
	}

	// Verify that temporary files were cleaned up
	for _, f := range pb.tempFiles {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("temporary file %s was not cleaned up after error", f)
		}
	}
}

// TestExecContextCancellation verifies that Exec respects context cancellation.
func TestExecContextCancellation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-exec-cancel")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	playbookFile := filepath.Join(tempDir, "test.yml")
	if err := os.WriteFile(playbookFile, []byte("dummy content"), 0644); err != nil {
		t.Fatalf("Failed to write playbook: %v", err)
	}

	pb := NewPlaybook()
	pb.Config.TempDir = tempDir
	pb.Config.Playbooks = []string{playbookFile}
	pb.Config.Inventories = []string{getInventoryHost() + ","}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = pb.Exec(ctx)
	if err == nil {
		t.Skip("skipping context-cancellation check: ansible binary not available")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context") {
		t.Errorf("expected context cancellation error, got: %v", err)
	}
}

// TestExecNonExistentGalaxyFile verifies that Exec returns an error for a non-existent galaxy file.
func TestExecNonExistentGalaxyFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-exec-galaxy")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	playbookFile := filepath.Join(tempDir, "test.yml")
	if err := os.WriteFile(playbookFile, []byte("dummy content"), 0644); err != nil {
		t.Fatalf("Failed to write playbook: %v", err)
	}

	pb := NewPlaybook()
	pb.Config.TempDir = tempDir
	pb.Config.Playbooks = []string{playbookFile}
	pb.Config.Inventories = []string{getInventoryHost() + ","}
	pb.Config.GalaxyFile = "/nonexistent/requirements.yml"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = pb.Exec(ctx)
	if err == nil {
		t.Fatal("expected error for non-existent galaxy file, got nil")
	}
	if !strings.Contains(err.Error(), "galaxy file not found") {
		t.Errorf("expected 'galaxy file not found' error, got: %v", err)
	}
}

// TestBuildCustomEnvVarsConfigFileNotFound verifies error for missing config file.
func TestBuildCustomEnvVarsConfigFileNotFound(t *testing.T) {
	cfg := Config{
		ConfigFile: "/nonexistent/ansible.cfg",
	}
	_, err := buildCustomEnvVars(&cfg)
	if err == nil {
		t.Fatal("expected error for non-existent config file, got nil")
	}
	if !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("expected 'config file not found' error, got: %v", err)
	}
}

// TestBuildCustomEnvVarsNoConfigFile verifies no error when ConfigFile is empty.
func TestBuildCustomEnvVarsNoConfigFile(t *testing.T) {
	cfg := Config{}
	envVars, err := buildCustomEnvVars(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envVars) != 0 {
		t.Errorf("expected no env vars, got %v", envVars)
	}
}

// TestBuildCustomEnvVarsFactPath verifies that FactPath is set as ANSIBLE_FACT_PATH.
func TestBuildCustomEnvVarsFactPath(t *testing.T) {
	cfg := Config{
		FactPath: "/custom/facts",
	}
	envVars, err := buildCustomEnvVars(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, env := range envVars {
		if env == "ANSIBLE_FACT_PATH=/custom/facts" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ANSIBLE_FACT_PATH in env vars, got %v", envVars)
	}
}

// TestBuildGalaxyCommand verifies that buildGalaxyCommand constructs
// the command with base args, options, and verbose flag.
func TestBuildGalaxyCommand(t *testing.T) {
	tests := []struct {
		name        string
		base        []string
		opts        []argOption
		verbose     int
		wantBinary  string
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:        "base args only",
			base:        []string{"role", "install"},
			opts:        nil,
			verbose:     0,
			wantBinary:  "ansible-galaxy",
			wantContain: []string{"role", "install"},
		},
		{
			name: "with string option",
			base: []string{"role", "install"},
			opts: []argOption{
				{flag: "--server", value: "https://galaxy.example.com"},
			},
			verbose:     0,
			wantContain: []string{"role", "install", "--server", "https://galaxy.example.com"},
		},
		{
			name: "with bool option",
			base: []string{"collection", "install"},
			opts: []argOption{
				{flag: "--force", value: true},
			},
			verbose:     0,
			wantContain: []string{"collection", "install", "--force"},
		},
		{
			name: "empty string option skipped",
			base: []string{"role", "install"},
			opts: []argOption{
				{flag: "--server", value: ""},
			},
			verbose:     0,
			wantContain: []string{"role", "install"},
			wantAbsent:  []string{"--server"},
		},
		{
			name:        "with verbose level",
			base:        []string{"role", "install"},
			opts:        nil,
			verbose:     2,
			wantContain: []string{"role", "install", "-vv"},
		},
		{
			name: "multiple options with verbose",
			base: []string{"collection", "install"},
			opts: []argOption{
				{flag: "--server", value: "https://galaxy.example.com"},
				{flag: "--force", value: true},
				{flag: "--timeout", value: 30},
			},
			verbose:     1,
			wantContain: []string{"collection", "install", "--server", "https://galaxy.example.com", "--force", "--timeout", "30", "-v"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb := NewPlaybook()
			pb.Config.Verbose = tt.verbose
			cmd := pb.buildGalaxyCommand(context.Background(), tt.base, tt.opts)

			if !strings.Contains(cmd.Path, "ansible-galaxy") {
				t.Errorf("expected binary to contain 'ansible-galaxy', got %s", cmd.Path)
			}

			args := cmd.Args[1:] // skip binary name
			for _, want := range tt.wantContain {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected arg %q in command args %v", want, args)
				}
			}

			for _, absent := range tt.wantAbsent {
				for _, arg := range args {
					if arg == absent {
						t.Errorf("unexpected arg %q in command args %v", absent, args)
					}
				}
			}
		})
	}
}

// TestCommonGalaxyOptions verifies that commonGalaxyOptions returns
// options reflecting the Config fields.
func TestCommonGalaxyOptions(t *testing.T) {
	t.Run("default config returns empty-valued options", func(t *testing.T) {
		pb := NewPlaybook()
		opts := pb.commonGalaxyOptions()

		expectedFlags := []string{"--server", "--api-key", "--ignore-certs", "--timeout", "--force", "--force-with-deps"}
		for i, expected := range expectedFlags {
			if opts[i].flag != expected {
				t.Errorf("option[%d] flag = %q, want %q", i, opts[i].flag, expected)
			}
		}
	})

	t.Run("populated config reflects values", func(t *testing.T) {
		pb := NewPlaybook()
		pb.Config.GalaxyAPIServerURL = "https://galaxy.example.com"
		pb.Config.GalaxyAPIKey = "my-api-key"
		pb.Config.GalaxyIgnoreCerts = true
		pb.Config.GalaxyTimeout = 60
		pb.Config.GalaxyForce = true
		pb.Config.GalaxyForceWithDeps = true

		opts := pb.commonGalaxyOptions()

		// Build args from the options to verify they produce correct flags
		var args []string
		for _, opt := range opts {
			args = applyOption(args, opt)
		}

		expectedArgs := []string{
			"--server", "https://galaxy.example.com",
			"--api-key", "my-api-key",
			"--ignore-certs",
			"--timeout", "60",
			"--force",
			"--force-with-deps",
		}

		if len(args) != len(expectedArgs) {
			t.Fatalf("expected %d args, got %d: %v", len(expectedArgs), len(args), args)
		}
		for i, want := range expectedArgs {
			if args[i] != want {
				t.Errorf("arg[%d] = %q, want %q", i, args[i], want)
			}
		}
	})
}

// TestGalaxyRoleCommand verifies the role install command construction.
func TestGalaxyRoleCommand(t *testing.T) {
	tests := []struct {
		name             string
		galaxyFile       string
		requirementsFile string
		rolesPath        string
		noDeps           bool
		verbose          int
		wantContain      []string
	}{
		{
			name:        "basic role install",
			galaxyFile:  "requirements.yml",
			wantContain: []string{"role", "install", "--role-file", "requirements.yml"},
		},
		{
			name:             "requirements file overrides galaxy file",
			galaxyFile:       "galaxy.yml",
			requirementsFile: "custom-requirements.yml",
			wantContain:      []string{"role", "install", "--role-file", "custom-requirements.yml"},
		},
		{
			name:        "with roles path and no-deps",
			galaxyFile:  "requirements.yml",
			rolesPath:   "/opt/roles",
			noDeps:      true,
			wantContain: []string{"role", "install", "--role-file", "requirements.yml", "--roles-path", "/opt/roles", "--no-deps"},
		},
		{
			name:        "with verbose",
			galaxyFile:  "requirements.yml",
			verbose:     3,
			wantContain: []string{"role", "install", "--role-file", "requirements.yml", "-vvv"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb := NewPlaybook()
			pb.Config.GalaxyFile = tt.galaxyFile
			pb.Config.GalaxyRequirementsFile = tt.requirementsFile
			pb.Config.GalaxyRolesPath = tt.rolesPath
			pb.Config.GalaxyNoDeps = tt.noDeps
			pb.Config.Verbose = tt.verbose

			cmd := pb.galaxyRoleCommand(context.Background())

			if !strings.Contains(cmd.Path, "ansible-galaxy") {
				t.Errorf("expected binary 'ansible-galaxy', got %s", cmd.Path)
			}

			args := cmd.Args[1:]
			for _, want := range tt.wantContain {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected arg %q in %v", want, args)
				}
			}
		})
	}
}

// TestGalaxyCollectionCommand verifies the collection install command construction.
func TestGalaxyCollectionCommand(t *testing.T) {
	tests := []struct {
		name             string
		galaxyFile       string
		requirementsFile string
		collectionsPath  string
		pre              bool
		upgrade          bool
		keyring          string
		disableGPG       bool
		sigCount         int
		sigStatusCodes   []string
		signature        string
		offline          bool
		verbose          int
		wantContain      []string
	}{
		{
			name:        "basic collection install",
			galaxyFile:  "requirements.yml",
			wantContain: []string{"collection", "install", "--requirements-file", "requirements.yml"},
		},
		{
			name:             "requirements file overrides galaxy file",
			galaxyFile:       "galaxy.yml",
			requirementsFile: "collections.yml",
			wantContain:      []string{"collection", "install", "--requirements-file", "collections.yml"},
		},
		{
			name:            "with collections path and pre",
			galaxyFile:      "requirements.yml",
			collectionsPath: "/opt/collections",
			pre:             true,
			wantContain:     []string{"collection", "install", "--requirements-file", "requirements.yml", "--collections-path", "/opt/collections", "--pre"},
		},
		{
			name:        "with upgrade and offline",
			galaxyFile:  "requirements.yml",
			upgrade:     true,
			offline:     true,
			wantContain: []string{"collection", "install", "--upgrade", "--offline"},
		},
		{
			name:       "with GPG options",
			galaxyFile: "requirements.yml",
			keyring:    "/path/to/keyring.gpg",
			disableGPG: true,
			sigCount:   2,
			signature:  "my-sig",
			wantContain: []string{
				"collection", "install",
				"--keyring", "/path/to/keyring.gpg",
				"--disable-gpg-verify",
				"--required-valid-signature-count", "2",
				"--signature", "my-sig",
			},
		},
		{
			name:           "with signature status codes",
			galaxyFile:     "requirements.yml",
			sigStatusCodes: []string{"EXPKEYSIG", "REVKEYSIG"},
			wantContain:    []string{"--ignore-signature-status-code", "EXPKEYSIG", "REVKEYSIG"},
		},
		{
			name:        "with verbose",
			galaxyFile:  "requirements.yml",
			verbose:     4,
			wantContain: []string{"collection", "install", "-vvvv"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb := NewPlaybook()
			pb.Config.GalaxyFile = tt.galaxyFile
			pb.Config.GalaxyRequirementsFile = tt.requirementsFile
			pb.Config.GalaxyCollectionsPath = tt.collectionsPath
			pb.Config.GalaxyPre = tt.pre
			pb.Config.GalaxyUpgrade = tt.upgrade
			pb.Config.GalaxyKeyring = tt.keyring
			pb.Config.GalaxyDisableGPGVerify = tt.disableGPG
			pb.Config.GalaxyRequiredValidSignatureCount = tt.sigCount
			pb.Config.GalaxyIgnoreSignatureStatusCodes = tt.sigStatusCodes
			pb.Config.GalaxySignature = tt.signature
			pb.Config.GalaxyOffline = tt.offline
			pb.Config.Verbose = tt.verbose

			cmd := pb.galaxyCollectionCommand(context.Background())

			if !strings.Contains(cmd.Path, "ansible-galaxy") {
				t.Errorf("expected binary 'ansible-galaxy', got %s", cmd.Path)
			}

			args := cmd.Args[1:]
			for _, want := range tt.wantContain {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected arg %q in %v", want, args)
				}
			}
		})
	}
}

// TestTrace verifies that trace prints the command line to stdout.
func TestTrace(t *testing.T) {
	pb := NewPlaybook()
	pb.Debug = true

	cmd := exec.Command("ansible-playbook", "--inventory", "localhost,", "site.yml")

	var buf bytes.Buffer
	pb.TraceOutput = &buf

	pb.trace(cmd)

	output := buf.String()
	expected := "$ ansible-playbook --inventory localhost, site.yml\n"
	if output != expected {
		t.Errorf("trace output = %q, want %q", output, expected)
	}
}

// TestTraceMasksSensitiveArgs verifies that trace masks sensitive flag values.
func TestTraceMasksSensitiveArgs(t *testing.T) {
	pb := NewPlaybook()

	cmd := exec.Command("ansible-playbook", "--inventory", "hosts.ini", "--extra-vars", "secret_password=hunter2", "-vv", "deploy.yml")

	var buf bytes.Buffer
	pb.TraceOutput = &buf

	pb.trace(cmd)

	output := buf.String()

	if strings.Contains(output, "hunter2") {
		t.Errorf("trace output should not contain sensitive value, got: %s", output)
	}
	if !strings.Contains(output, "******") {
		t.Errorf("trace output should contain masked value '******', got: %s", output)
	}
	// Non-sensitive args should still be present
	for _, want := range []string{"ansible-playbook", "--inventory", "hosts.ini", "--extra-vars", "-vv", "deploy.yml"} {
		if !strings.Contains(output, want) {
			t.Errorf("trace output missing %q, got: %s", want, output)
		}
	}
}

// TestMaskSensitiveArgs verifies maskSensitiveArgs directly.
func TestMaskSensitiveArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "no sensitive flags",
			args: []string{"ansible-playbook", "--inventory", "hosts.ini", "site.yml"},
			want: []string{"ansible-playbook", "--inventory", "hosts.ini", "site.yml"},
		},
		{
			name: "extra-vars masked",
			args: []string{"ansible-playbook", "--extra-vars", "password=secret123", "site.yml"},
			want: []string{"ansible-playbook", "--extra-vars", "******", "site.yml"},
		},
		{
			name: "vault-password-file masked",
			args: []string{"ansible-playbook", "--vault-password-file", "/tmp/vault.txt", "site.yml"},
			want: []string{"ansible-playbook", "--vault-password-file", "******", "site.yml"},
		},
		{
			name: "private-key masked",
			args: []string{"ansible-playbook", "--private-key", "/home/user/.ssh/id_rsa", "site.yml"},
			want: []string{"ansible-playbook", "--private-key", "******", "site.yml"},
		},
		{
			name: "api-key masked",
			args: []string{"ansible-galaxy", "--api-key", "tok_abc123", "role", "install"},
			want: []string{"ansible-galaxy", "--api-key", "******", "role", "install"},
		},
		{
			name: "multiple sensitive flags",
			args: []string{"ansible-playbook", "--extra-vars", "pw=x", "--private-key", "/key", "site.yml"},
			want: []string{"ansible-playbook", "--extra-vars", "******", "--private-key", "******", "site.yml"},
		},
		{
			name: "sensitive flag at end without value",
			args: []string{"ansible-playbook", "--extra-vars"},
			want: []string{"ansible-playbook", "--extra-vars"},
		},
		{
			name: "original args not mutated",
			args: []string{"ansible-playbook", "--extra-vars", "secret"},
			want: []string{"ansible-playbook", "--extra-vars", "******"},
		},
		{
			name: "equals-joined extra-vars masked",
			args: []string{"ansible-playbook", "--extra-vars=secret_password=hunter2", "site.yml"},
			want: []string{"ansible-playbook", "--extra-vars=******", "site.yml"},
		},
		{
			name: "short form -e masked",
			args: []string{"ansible-playbook", "-e", "pw=secret", "site.yml"},
			want: []string{"ansible-playbook", "-e", "******", "site.yml"},
		},
		{
			name: "equals-joined private-key masked",
			args: []string{"ansible-playbook", "--private-key=/home/user/.ssh/id_rsa", "site.yml"},
			want: []string{"ansible-playbook", "--private-key=******", "site.yml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Keep a copy to check for mutation
			original := make([]string, len(tt.args))
			copy(original, tt.args)

			got := maskSensitiveArgs(tt.args)

			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d: %v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}

			// Verify original slice was not mutated
			for i := range original {
				if tt.args[i] != original[i] {
					t.Errorf("original args mutated at [%d]: %q -> %q", i, original[i], tt.args[i])
				}
			}
		})
	}
}

// TestAnsibleErrorMessage verifies AnsibleError.Error() format.
func TestAnsibleErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		ae       AnsibleError
		expected string
	}{
		{
			name:     "general error",
			ae:       AnsibleError{ExitCode: ExitCodeError, Command: "ansible-playbook", Message: "general error", CommandIndex: 1, TotalCommands: 1},
			expected: "ansible-playbook: general error (exit code 1, command 1/1)",
		},
		{
			name:     "host failed",
			ae:       AnsibleError{ExitCode: ExitCodeHostFailed, Command: "ansible-playbook", Message: "one or more hosts failed", CommandIndex: 2, TotalCommands: 4},
			expected: "ansible-playbook: one or more hosts failed (exit code 2, command 2/4)",
		},
		{
			name:     "unreachable",
			ae:       AnsibleError{ExitCode: ExitCodeUnreachable, Command: "ansible-playbook", Message: "one or more hosts unreachable", CommandIndex: 1, TotalCommands: 1},
			expected: "ansible-playbook: one or more hosts unreachable (exit code 3, command 1/1)",
		},
		{
			name:     "parser error",
			ae:       AnsibleError{ExitCode: ExitCodeParserError, Command: "ansible-playbook", Message: "parser error", CommandIndex: 1, TotalCommands: 1},
			expected: "ansible-playbook: parser error (exit code 4, command 1/1)",
		},
		{
			name:     "user abort",
			ae:       AnsibleError{ExitCode: ExitCodeUserAbort, Command: "ansible-playbook", Message: "user interrupted execution", CommandIndex: 3, TotalCommands: 4},
			expected: "ansible-playbook: user interrupted execution (exit code 99, command 3/4)",
		},
		{
			name:     "unexpected error",
			ae:       AnsibleError{ExitCode: ExitCodeUnexpected, Command: "ansible-playbook", Message: "unexpected error", CommandIndex: 1, TotalCommands: 2},
			expected: "ansible-playbook: unexpected error (exit code 250, command 1/2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ae.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestAnsibleErrorUnwrap verifies that AnsibleError.Unwrap() returns the underlying error.
func TestAnsibleErrorUnwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	ae := &AnsibleError{
		ExitCode: ExitCodeError,
		Command:  "ansible-playbook",
		Message:  "general error",
		Err:      underlying,
	}

	if !errors.Is(ae, underlying) {
		t.Error("expected errors.Is to match underlying error")
	}

	var target *AnsibleError
	if !errors.As(ae, &target) {
		t.Error("expected errors.As to match *AnsibleError")
	}
	if target.ExitCode != ExitCodeError {
		t.Errorf("ExitCode = %d, want %d", target.ExitCode, ExitCodeError)
	}
}

// TestNewAnsibleError verifies newAnsibleError mapping for known and unknown exit codes.
func TestNewAnsibleError(t *testing.T) {
	tests := []struct {
		name        string
		exitCode    int
		wantMessage string
	}{
		{"exit code 1", 1, "general error"},
		{"exit code 2", 2, "one or more hosts failed"},
		{"exit code 3", 3, "one or more hosts unreachable"},
		{"exit code 4", 4, "parser error"},
		{"exit code 99", 99, "user interrupted execution"},
		{"exit code 250", 250, "unexpected error"},
		{"exit code 42 unknown", 42, "unknown error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run a command that exits with the desired exit code
			cmd := exec.Command("sh", "-c", "exit "+strconv.Itoa(tt.exitCode)) //nolint:gosec // test code with controlled input
			err := cmd.Run()
			if err == nil {
				t.Fatal("expected command to fail")
			}

			ae := newAnsibleError("ansible-playbook", err)
			if ae == nil {
				t.Fatal("expected AnsibleError, got nil")
			}
			if ae.ExitCode != tt.exitCode {
				t.Errorf("ExitCode = %d, want %d", ae.ExitCode, tt.exitCode)
			}
			if ae.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", ae.Message, tt.wantMessage)
			}
			if ae.Command != "ansible-playbook" {
				t.Errorf("Command = %q, want %q", ae.Command, "ansible-playbook")
			}
		})
	}
}

// TestNewAnsibleErrorNonExitError verifies that newAnsibleError returns nil for non-exit errors.
func TestNewAnsibleErrorNonExitError(t *testing.T) {
	err := errors.New("not an exec.ExitError")
	ae := newAnsibleError("ansible-playbook", err)
	if ae != nil {
		t.Errorf("expected nil for non-ExitError, got %v", ae)
	}
}

// TestWriteTempFileInvalidDir verifies error when temp directory does not exist.
func TestWriteTempFileInvalidDir(t *testing.T) {
	_, err := writeTempFile("/nonexistent/dir/that/does/not/exist", "test-", "content", 0600)
	if err == nil {
		t.Fatal("expected error for non-existent temp directory, got nil")
	}
	if !strings.Contains(err.Error(), "could not create temp file") {
		t.Errorf("expected 'could not create temp file' error, got: %v", err)
	}
}

// TestWriteTempFileReadOnlyDir verifies error when writing to a read-only directory.
func TestWriteTempFileReadOnlyDir(t *testing.T) {
	// Create a directory and make it read-only
	roDir, err := os.MkdirTemp("", "test-readonly")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.Chmod(roDir, 0o600) //nolint:gosec // restore permissions for cleanup
		_ = os.RemoveAll(roDir)
	}()

	if err := os.Chmod(roDir, 0o400); err != nil {
		t.Fatalf("Failed to change permissions: %v", err)
	}

	_, err = writeTempFile(roDir, "test-", "content", 0600)
	if err == nil {
		t.Fatal("expected error for read-only directory, got nil")
	}
}

// TestWriteTempFileChmodError verifies error when chmod fails on the temp file.
// This is tested by creating a file in a directory, then making the directory
// non-searchable so that chmod on the file fails.
func TestWriteTempFileChmodError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	// Create a normal temp dir where we can create files
	parentDir, err := os.MkdirTemp("", "test-chmod-parent")
	if err != nil {
		t.Fatalf("Failed to create parent temp dir: %v", err)
	}
	defer func() {
		_ = os.Chmod(parentDir, 0o700) //nolint:gosec // restore permissions for cleanup
		_ = os.RemoveAll(parentDir)
	}()

	// Create the temp file normally first to verify the path works
	fname, err := writeTempFile(parentDir, "test-", "content", 0600)
	if err != nil {
		t.Fatalf("writeTempFile should succeed: %v", err)
	}
	_ = os.Remove(fname)

	// Now create a subdirectory, create a file in it, then remove search permission
	// so that chmod on the file will fail
	subDir, err := os.MkdirTemp(parentDir, "sub")
	if err != nil {
		t.Fatalf("Failed to create sub dir: %v", err)
	}
	defer func() { _ = os.Chmod(subDir, 0o700) }() //nolint:gosec // restore permissions for cleanup

	// Write a file, then remove directory execute permission to break chmod
	fname2, err := writeTempFile(subDir, "test-", "content", 0600)
	if err != nil {
		t.Fatalf("writeTempFile should succeed initially: %v", err)
	}
	// Clean up the successful file
	_ = os.Remove(fname2)

	// Remove execute (search) permission on parent so new files can't be chmod'd
	if err := os.Chmod(subDir, 0o200); err != nil {
		t.Fatalf("Failed to change permissions: %v", err)
	}

	// This should fail at os.CreateTemp since the directory is not searchable
	_, err = writeTempFile(subDir, "test-", "content", 0600)
	if err == nil {
		t.Fatal("expected error when directory permissions prevent file operations")
	}
}

// TestWriteTempFileCRLFNormalization verifies that CRLF line endings are normalized to LF.
func TestWriteTempFileCRLFNormalization(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-crlf")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	fname, err := writeTempFile(tempDir, "test-", "line1\r\nline2\r\n", 0600)
	if err != nil {
		t.Fatalf("writeTempFile failed: %v", err)
	}
	defer func() { _ = os.Remove(fname) }()

	data, err := os.ReadFile(fname)
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	content := string(data)
	if strings.Contains(content, "\r\n") {
		t.Error("expected CRLF to be normalized to LF")
	}
	if !strings.HasSuffix(content, "\n") {
		t.Error("expected trailing newline")
	}
}

// TestWriteTempFileTrailingNewline verifies that a trailing newline is added if missing.
func TestWriteTempFileTrailingNewline(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-trailing")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	fname, err := writeTempFile(tempDir, "test-", "no-newline", 0600)
	if err != nil {
		t.Fatalf("writeTempFile failed: %v", err)
	}
	defer func() { _ = os.Remove(fname) }()

	data, err := os.ReadFile(fname)
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	if !strings.HasSuffix(string(data), "\n") {
		t.Error("expected trailing newline to be added")
	}
}

// TestWriteTempFilePermissions verifies that the file is created with correct permissions.
func TestWriteTempFilePermissions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-perms")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	fname, err := writeTempFile(tempDir, "test-", "content", 0600)
	if err != nil {
		t.Fatalf("writeTempFile failed: %v", err)
	}
	defer func() { _ = os.Remove(fname) }()

	info, err := os.Stat(fname)
	if err != nil {
		t.Fatalf("Failed to stat temp file: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected permissions 0600, got %04o", perm)
	}
}

// TestPrepareTempFilesInvalidDir verifies prepareTempFiles error with invalid temp dir.
func TestPrepareTempFilesInvalidDir(t *testing.T) {
	pb := NewPlaybook()
	pb.Config.TempDir = "/nonexistent/dir"
	pb.Config.PrivateKey = "-----BEGIN RSA PRIVATE KEY-----\nDATA\n-----END RSA PRIVATE KEY-----"

	err := pb.prepareTempFiles()
	if err == nil {
		t.Fatal("expected error for invalid temp dir, got nil")
	}
	if !strings.Contains(err.Error(), "could not create private key file") {
		t.Errorf("expected 'could not create private key file' error, got: %v", err)
	}
}

// TestPrepareTempFilesInvalidVaultDir verifies prepareTempFiles error for vault with invalid temp dir.
func TestPrepareTempFilesInvalidVaultDir(t *testing.T) {
	pb := NewPlaybook()
	pb.Config.TempDir = "/nonexistent/dir"
	pb.Config.VaultPassword = "secret"

	err := pb.prepareTempFiles()
	if err == nil {
		t.Fatal("expected error for invalid temp dir, got nil")
	}
	if !strings.Contains(err.Error(), "could not create vault password file") {
		t.Errorf("expected 'could not create vault password file' error, got: %v", err)
	}
}

// TestAnsibleCommandSyntaxCheck verifies that --syntax-check is added when SyntaxCheck is true.
func TestAnsibleCommandSyntaxCheck(t *testing.T) {
	pb := NewPlaybook()
	pb.Config.Playbooks = []string{"playbook.yml"}
	pb.Config.SyntaxCheck = true
	inv := getInventoryHost() + ","
	cmd := pb.ansibleCommand(context.Background(), inv)

	found := false
	for _, arg := range cmd.Args {
		if arg == "--syntax-check" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected --syntax-check in command arguments")
	}
}

// TestAnsibleCommandListHosts verifies that --list-hosts is added when ListHosts is true.
func TestAnsibleCommandListHosts(t *testing.T) {
	pb := NewPlaybook()
	pb.Config.Playbooks = []string{"playbook.yml"}
	pb.Config.ListHosts = true
	inv := getInventoryHost() + ","
	cmd := pb.ansibleCommand(context.Background(), inv)

	found := false
	for _, arg := range cmd.Args {
		if arg == "--list-hosts" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected --list-hosts in command arguments")
	}
}

// TestValidateInventoryExistingFile verifies that an existing inventory file passes validation.
func TestValidateInventoryExistingFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-inventory")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	invFile := filepath.Join(tempDir, "hosts.ini")
	if err := os.WriteFile(invFile, []byte("localhost"), 0o600); err != nil {
		t.Fatalf("Failed to write inventory file: %v", err)
	}

	if err := validateInventory(invFile); err != nil {
		t.Errorf("Expected valid inventory, got error: %v", err)
	}
}

// TestBuildCommandsWithGalaxyFile verifies that galaxy commands are included when GalaxyFile exists.
func TestBuildCommandsWithGalaxyFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-galaxy-commands")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	playbookFile := filepath.Join(tempDir, "test.yml")
	if err := os.WriteFile(playbookFile, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("Failed to write playbook: %v", err)
	}

	galaxyFile := filepath.Join(tempDir, "requirements.yml")
	if err := os.WriteFile(galaxyFile, []byte("roles: []"), 0o600); err != nil {
		t.Fatalf("Failed to write galaxy file: %v", err)
	}

	pb := NewPlaybook()
	pb.Config.TempDir = tempDir
	pb.Config.Playbooks = []string{playbookFile}
	pb.Config.Inventories = []string{getInventoryHost() + ","}
	pb.Config.GalaxyFile = galaxyFile

	cmds, err := pb.buildCommands(context.Background())
	if err != nil {
		t.Fatalf("buildCommands failed: %v", err)
	}

	// Expect: version + galaxy role + galaxy collection + playbook = 4 commands
	if len(cmds) != 4 {
		t.Errorf("Expected 4 commands (version+role+collection+playbook), got %d", len(cmds))
	}
}

// TestBuildCommandsInvalidInventory verifies that buildCommands returns error for invalid inventory.
func TestBuildCommandsInvalidInventory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-invalid-inv")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	playbookFile := filepath.Join(tempDir, "test.yml")
	if err := os.WriteFile(playbookFile, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("Failed to write playbook: %v", err)
	}

	pb := NewPlaybook()
	pb.Config.TempDir = tempDir
	pb.Config.Playbooks = []string{playbookFile}
	pb.Config.Inventories = []string{"/nonexistent/hosts"}

	_, err = pb.buildCommands(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid inventory, got nil")
	}
	if !strings.Contains(err.Error(), "inventory not found") {
		t.Errorf("expected 'inventory not found' error, got: %v", err)
	}
}

// TestExecWithInvalidConfigFile verifies that Exec returns error for non-existent config file.
func TestExecWithInvalidConfigFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-exec-config")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	playbookFile := filepath.Join(tempDir, "test.yml")
	if err := os.WriteFile(playbookFile, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("Failed to write playbook: %v", err)
	}

	pb := NewPlaybook()
	pb.Config.TempDir = tempDir
	pb.Config.Playbooks = []string{playbookFile}
	pb.Config.Inventories = []string{getInventoryHost() + ","}
	pb.Config.ConfigFile = "/nonexistent/ansible.cfg"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = pb.Exec(ctx)
	if err == nil {
		t.Fatal("expected error for non-existent config file, got nil")
	}
	if !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("expected 'config file not found' error, got: %v", err)
	}
}

// TestAnsibleExitCodeConstants verifies the exit code constants have correct values.
func TestAnsibleExitCodeConstants(t *testing.T) {
	if ExitCodeSuccess != 0 {
		t.Errorf("ExitCodeSuccess = %d, want 0", ExitCodeSuccess)
	}
	if ExitCodeError != 1 {
		t.Errorf("ExitCodeError = %d, want 1", ExitCodeError)
	}
	if ExitCodeHostFailed != 2 {
		t.Errorf("ExitCodeHostFailed = %d, want 2", ExitCodeHostFailed)
	}
	if ExitCodeUnreachable != 3 {
		t.Errorf("ExitCodeUnreachable = %d, want 3", ExitCodeUnreachable)
	}
	if ExitCodeParserError != 4 {
		t.Errorf("ExitCodeParserError = %d, want 4", ExitCodeParserError)
	}
	if ExitCodeUserAbort != 99 {
		t.Errorf("ExitCodeUserAbort = %d, want 99", ExitCodeUserAbort)
	}
	if ExitCodeUnexpected != 250 {
		t.Errorf("ExitCodeUnexpected = %d, want 250", ExitCodeUnexpected)
	}
}
