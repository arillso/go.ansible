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

// TestPrepareTempFiles tests the creation of temporary files (PrivateKey and VaultPassword).
func TestPrepareTempFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-temp")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	pb := NewPlaybook()
	pb.Config.TempDir = tempDir

	privateKeyContent := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC..."
	vaultPasswordContent := "my_vault_password"
	pb.Config.PrivateKey = privateKeyContent
	pb.Config.VaultPassword = vaultPasswordContent

	if err := pb.prepareTempFiles(); err != nil {
		t.Fatalf("prepareTempFiles failed: %v", err)
	}

	// Verify the content of the private key file.
	data, err := os.ReadFile(pb.Config.PrivateKeyFile)
	if err != nil {
		t.Fatalf("Failed to read the private key file: %v", err)
	}
	if string(data) != privateKeyContent {
		t.Errorf("Private key file content mismatch, expected %q, got %q", privateKeyContent, string(data))
	}

	// Verify the content of the vault password file.
	data, err = os.ReadFile(pb.Config.VaultPasswordFile)
	if err != nil {
		t.Fatalf("Failed to read the vault password file: %v", err)
	}
	if string(data) != vaultPasswordContent {
		t.Errorf("Vault password file content mismatch, expected %q, got %q", vaultPasswordContent, string(data))
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
	if string(data) != content {
		t.Errorf("Expected content %q, got: %q", content, string(data))
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
