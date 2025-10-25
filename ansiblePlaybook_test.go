// ansiblePlaybook_test.go
// Description: Comprehensive tests for the ansible package.
// Author: Your Name
// These tests cover functions such as:
// - resolvePlaybooks: Resolving playbook file paths from patterns.
// - prepareTempFiles & cleanupTempFiles: Creating and cleaning up temporary files.
// - buildCustomEnvVars: Assembling additional environment variables.
// - Other helper functions like addVerbose, appendExtraVars, and writeTempFile.

package ansible

import (
	"context"
	"os"
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
	defer os.RemoveAll(tempDir)

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
	defer os.RemoveAll(tempDir)

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
	defer os.RemoveAll(tempDir)

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
	defer os.RemoveAll(tempDir)

	cfg := Config{}
	// Create a dummy configuration file.
	cfgFilePath := filepath.Join(tempDir, "ansible.cfg")
	if err := os.WriteFile(cfgFilePath, []byte("dummy config"), 0644); err != nil {
		t.Fatalf("Failed to write dummy configuration file: %v", err)
	}
	cfg.ConfigFile = cfgFilePath
	cfg.FactCaching = "jsonfile"
	cfg.FactCachingTimeout = 60

	envVars := buildCustomEnvVars(cfg)
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
	args := []string{"test"}
	args = addVerbose(args, 3)
	// The last element should be "-vvv".
	if len(args) == 0 {
		t.Errorf("Expected non-empty argument list")
	}
	if args[len(args)-1] != "-vvv" {
		t.Errorf("Expected \"-vvv\", got: %s", args[len(args)-1])
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
	defer os.RemoveAll(tempDir)

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
	defer os.RemoveAll(tempDir)

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
			defer os.RemoveAll(tempDir)

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
			defer os.RemoveAll(tempDir)

			file, err := writeTempFile(tempDir, tt.prefix, tt.content, 0600)
			if (err != nil) != tt.wantErr {
				t.Errorf("writeTempFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if file != "" {
				defer os.Remove(file)

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

// TestExec simulates a call to Exec without actually executing external commands
// by using a short timeout context. Note: This test focuses on flow and error handling.
func TestExec(t *testing.T) {
	pb := NewPlaybook()
	// Set a dummy playbook; the content is not important for this test.
	pb.Config.Playbooks = []string{"playbook.yml"}
	// Use inline inventory.
	pb.Config.Inventories = []string{getInventoryHost() + ","}

	// Create a context with timeout to ensure the test terminates.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Since actual external commands may not be executed in the test environment,
	// we expect either quick error handling or a timeout.
	err := pb.Exec(ctx)
	if err != nil {
		t.Logf("pb.Exec returned an expected error: %v", err)
	} else {
		t.Log("pb.Exec executed without error (this might be unexpected in the test environment)")
	}
}
