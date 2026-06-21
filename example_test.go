// Runnable examples for the ansible package. These compile and run as part of
// `go test`, appear on pkg.go.dev, and use CommandStrings so they preview the
// generated command lines without needing an ansible binary installed.
package ansible_test

import (
	"context"
	"fmt"

	ansible "github.com/arillso/go.ansible/v2"
)

// ExampleNewPlaybook shows the minimal setup: run a playbook against an
// inline inventory.
func ExampleNewPlaybook() {
	pb := ansible.NewPlaybook()
	pb.Config.Playbooks = []string{"arillso.example.site"}
	pb.Config.Inventories = []string{"localhost,"}

	// Exec(ctx) would run it; CommandStrings previews the command instead.
	cmds, err := pb.CommandStrings(context.Background())
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(cmds[0])
	// Output: ansible-playbook --inventory localhost, --forks 5 arillso.example.site
}

// ExamplePlaybook_vault shows running a playbook with a Vault password. The
// password is written to a temporary file and passed via --vault-password-file;
// set TempDir to a tmpfs in security-critical environments.
func ExamplePlaybook_vault() {
	pb := ansible.NewPlaybook()
	pb.Config.Playbooks = []string{"arillso.example.site"}
	pb.Config.Inventories = []string{"production,"}
	pb.Config.VaultPassword = "s3cr3t"

	if err := pb.Exec(context.Background()); err != nil {
		// handle the error; omitted here for brevity
		_ = err
	}
}

// ExamplePlaybook_galaxy shows installing roles and collections from a
// requirements file before the playbook run.
func ExamplePlaybook_galaxy() {
	pb := ansible.NewPlaybook()
	pb.Config.GalaxyFile = "requirements.yml"
	pb.Config.Playbooks = []string{"arillso.example.site"}
	pb.Config.Inventories = []string{"localhost,"}

	if err := pb.Exec(context.Background()); err != nil {
		_ = err
	}
}

// ExamplePlaybook_extraVars shows passing extra variables and limiting the run
// to a subset of hosts and tags.
func ExamplePlaybook_extraVars() {
	pb := ansible.NewPlaybook()
	pb.Config.Playbooks = []string{"arillso.example.site"}
	pb.Config.Inventories = []string{"production,"}
	pb.Config.ExtraVars = []string{"env=staging", "version=1.2.3"}
	pb.Config.Limit = "web"
	pb.Config.Tags = "deploy"

	cmds, err := pb.CommandStrings(context.Background())
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(cmds[0])
	// Output: ansible-playbook --inventory production, --forks 5 --limit web --tags deploy --extra-vars env=staging --extra-vars version=1.2.3 arillso.example.site
}

// ExamplePlaybook_context shows cancelling a run with a context. Cancelling the
// context terminates the underlying ansible-playbook process.
func ExamplePlaybook_context() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pb := ansible.NewPlaybook()
	pb.Config.Playbooks = []string{"arillso.example.site"}
	pb.Config.Inventories = []string{"localhost,"}

	// In real code, cancel() from another goroutine (e.g. on SIGINT) to stop
	// the run; Exec returns a context error when cancelled.
	if err := pb.Exec(ctx); err != nil {
		_ = err
	}
}
