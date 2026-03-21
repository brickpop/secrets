//go:build integration

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// binary is the path to the compiled secrets binary.
var binary string

func TestMain(m *testing.M) {
	// Build the binary once for all integration tests
	tmp, err := os.MkdirTemp("", "secrets-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating temp dir: %v\n", err)
		os.Exit(1)
	}

	binary = filepath.Join(tmp, "secrets")
	out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "building binary: %v\n%s\n", err, out)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// runner is a test helper that runs the secrets binary in an isolated store.
type runner struct {
	t       *testing.T
	storeDir string
	workDir  string
	env      []string
}

func newRunner(t *testing.T) *runner {
	t.Helper()
	storeDir := t.TempDir()
	workDir := t.TempDir()
	return &runner{
		t:        t,
		storeDir: storeDir,
		workDir:  workDir,
		env: []string{
			"SECRETS_STORE_DIR=" + storeDir,
			"HOME=" + t.TempDir(), // isolate from real home
			"PATH=" + os.Getenv("PATH"),
		},
	}
}

func (r *runner) run(args ...string) (string, string, error) {
	r.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = r.workDir
	cmd.Env = r.env
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (r *runner) mustRun(args ...string) string {
	r.t.Helper()
	stdout, stderr, err := r.run(args...)
	if err != nil {
		r.t.Fatalf("secrets %s failed: %v\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, stdout, stderr)
	}
	return stdout
}

func (r *runner) mustFail(args ...string) (string, string) {
	r.t.Helper()
	stdout, stderr, err := r.run(args...)
	if err == nil {
		r.t.Fatalf("secrets %s should have failed\nstdout: %s\nstderr: %s", strings.Join(args, " "), stdout, stderr)
	}
	return stdout, stderr
}

// runWithStdin runs with the given stdin content.
func (r *runner) runWithStdin(stdin string, args ...string) (string, string, error) {
	r.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = r.workDir
	cmd.Env = r.env
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (r *runner) mustRunWithStdin(stdin string, args ...string) string {
	r.t.Helper()
	stdout, stderr, err := r.runWithStdin(stdin, args...)
	if err != nil {
		r.t.Fatalf("secrets %s failed: %v\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, stdout, stderr)
	}
	return stdout
}

// initNoPassphrase initializes a store with no passphrase.
func (r *runner) initNoPassphrase() {
	r.t.Helper()
	r.mustRunWithStdin("\n\n", "init")
}

// writeFile writes a file in the runner's work directory.
func (r *runner) writeFile(name, content string) {
	r.t.Helper()
	path := filepath.Join(r.workDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		r.t.Fatalf("creating dir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		r.t.Fatalf("writing %s: %v", name, err)
	}
}

// --- Tests ---

func TestIntegration_CRUDLifecycle(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	// set
	r.mustRun("set", "API_KEY", "my-secret-key")
	r.mustRun("set", "DB_URL", "postgres://localhost/db")
	r.mustRun("set", "TOKEN", "abc123")

	// get
	out := r.mustRun("get", "API_KEY")
	if out != "my-secret-key" {
		t.Fatalf("get API_KEY = %q, want %q", out, "my-secret-key")
	}

	// get - no trailing newline
	if strings.HasSuffix(out, "\n") {
		t.Fatal("get should not have trailing newline")
	}

	// ls
	out = r.mustRun("ls")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("ls returned %d lines, want 3: %v", len(lines), lines)
	}
	// Should be sorted
	if lines[0] != "API_KEY" || lines[1] != "DB_URL" || lines[2] != "TOKEN" {
		t.Fatalf("ls not sorted: %v", lines)
	}

	// rm
	r.mustRun("rm", "TOKEN", "--force")
	out = r.mustRun("ls")
	lines = strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("ls after rm returned %d lines, want 2", len(lines))
	}

	// get deleted key should fail
	r.mustFail("get", "TOKEN")

	// rm nonexistent should fail
	r.mustFail("rm", "NONEXISTENT", "--force")
}

func TestIntegration_SetOverwrite(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY", "first")
	r.mustRun("set", "KEY", "second")

	out := r.mustRun("get", "KEY")
	if out != "second" {
		t.Fatalf("get after overwrite = %q, want %q", out, "second")
	}
}

func TestIntegration_SpecialValues(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	cases := map[string]string{
		"SPACES":    "value with spaces",
		"EQUALS":    "key=value=extra",
		"URL":       "https://user:pass@host:8080/path?q=1&r=2",
		"HEX":       "0xdeadbeef0123456789abcdef",
	}

	for k, v := range cases {
		r.mustRun("set", k, v)
	}

	for k, want := range cases {
		got := r.mustRun("get", k)
		if got != want {
			t.Errorf("get %s = %q, want %q", k, got, want)
		}
	}
}

func TestIntegration_ExportPosix(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "RPC_URL", "https://rpc.example.com")
	r.mustRun("set", "PRIVATE_KEY", "0xTESTKEY")

	r.writeFile(".secrets.yaml", `project: test
keys:
  - RPC_URL
  - PRIVATE_KEY
`)

	out := r.mustRun("export", "-f", filepath.Join(r.workDir, ".secrets.yaml"))
	if !strings.Contains(out, "export RPC_URL=") {
		t.Fatalf("posix export missing RPC_URL: %s", out)
	}
	if !strings.Contains(out, "export PRIVATE_KEY=") {
		t.Fatalf("posix export missing PRIVATE_KEY: %s", out)
	}
}

func TestIntegration_ExportFish(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "MY_VAR", "hello world")

	r.writeFile(".secrets.yaml", `project: test
keys:
  - MY_VAR
`)

	out := r.mustRun("export", "-f", filepath.Join(r.workDir, ".secrets.yaml"), "--format", "fish")
	if !strings.Contains(out, "set -x MY_VAR") {
		t.Fatalf("fish export missing set -x: %s", out)
	}
}

func TestIntegration_ExportDotenv(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "MY_VAR", "hello world")

	r.writeFile(".secrets.yaml", `project: test
keys:
  - MY_VAR
`)

	out := r.mustRun("export", "-f", filepath.Join(r.workDir, ".secrets.yaml"), "--format", "dotenv")
	if !strings.Contains(out, "MY_VAR=") {
		t.Fatalf("dotenv export missing MY_VAR: %s", out)
	}
}

func TestIntegration_ExportMapFile(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "PRIVATE_KEY", "0xGLOBALKEY")

	r.writeFile(".secrets.yaml", `project: test
keys:
  - PROJECT_PK
`)
	r.writeFile(".secrets-map.yaml", `PROJECT_PK: PRIVATE_KEY
`)

	out := r.mustRun("export", "-f", filepath.Join(r.workDir, ".secrets.yaml"))
	if !strings.Contains(out, "export PROJECT_PK=") {
		t.Fatalf("mapped export missing PROJECT_PK: %s", out)
	}
	if !strings.Contains(out, "0xGLOBALKEY") {
		t.Fatalf("mapped export has wrong value: %s", out)
	}
}

func TestIntegration_ExportPartial(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "EXISTS", "value")

	r.writeFile(".secrets.yaml", `project: test
keys:
  - EXISTS
  - MISSING
`)

	// Without --partial, should fail
	r.mustFail("export", "-f", filepath.Join(r.workDir, ".secrets.yaml"))

	// With --partial, should succeed with empty value for MISSING
	out := r.mustRun("export", "-f", filepath.Join(r.workDir, ".secrets.yaml"), "--partial")
	if !strings.Contains(out, "EXISTS") {
		t.Fatalf("partial export missing EXISTS: %s", out)
	}
	if !strings.Contains(out, "MISSING") {
		t.Fatalf("partial export missing MISSING: %s", out)
	}
}

func TestIntegration_DumpAllFormats(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY1", "val1")
	r.mustRun("set", "KEY2", "val2")

	for _, format := range []string{"posix", "fish", "dotenv"} {
		out := r.mustRun("dump", "--format", format)
		if !strings.Contains(out, "KEY1") || !strings.Contains(out, "KEY2") {
			t.Errorf("dump --%s missing keys: %s", format, out)
		}
	}
}

func TestIntegration_AgentLifecycle(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "AGENT_KEY", "agent-value")

	// Start agent — socket is at SECRETS_STORE_DIR/agent.sock automatically
	r.mustRun("agent", "--ttl", "30s")

	// get via agent (binary finds socket via SECRETS_STORE_DIR)
	val := r.mustRun("get", "AGENT_KEY")
	if val != "agent-value" {
		t.Fatalf("get via agent = %q, want %q", val, "agent-value")
	}

	// ls via agent
	lsOut := r.mustRun("ls")
	if !strings.Contains(lsOut, "AGENT_KEY") {
		t.Fatalf("ls via agent missing key: %s", lsOut)
	}

	// stop
	r.mustRun("agent", "stop")

	// Give it a moment to shut down
	time.Sleep(100 * time.Millisecond)

	// After stop, get should fall back to file
	val = r.mustRun("get", "AGENT_KEY")
	if val != "agent-value" {
		t.Fatalf("get after agent stop = %q, want %q", val, "agent-value")
	}
}

func TestIntegration_AgentAlreadyRunning(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY", "value")

	// Start agent
	r.mustRun("agent", "--ttl", "30s")

	// Second start should succeed (idempotent)
	_, stderr, err := r.run("agent")
	if err != nil {
		t.Fatalf("second agent start should not error: %v", err)
	}
	if !strings.Contains(stderr, "already running") {
		t.Fatalf("second agent start should say already running, got: %s", stderr)
	}

	// Clean up
	r.mustRun("agent", "stop")
	time.Sleep(100 * time.Millisecond)
}

func TestIntegration_Passwd_EmptyToSet(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "KEY", "value")

	// Change from empty to "newpass"
	r.mustRunWithStdin("newpass\nnewpass\n", "passwd")

	// get should now require passphrase — will fail without stdin
	_, _, err := r.run("get", "KEY")
	if err == nil {
		t.Fatal("get should fail without passphrase after passwd")
	}

	// get with passphrase via stdin
	out, _, err := r.runWithStdin("newpass\n", "get", "KEY")
	if err != nil {
		t.Fatalf("get with passphrase failed: %v", err)
	}
	if out != "value" {
		t.Fatalf("get = %q, want %q", out, "value")
	}
}

func TestIntegration_Passwd_SetToEmpty(t *testing.T) {
	r := newRunner(t)

	// Init with passphrase
	r.mustRunWithStdin("mypass\nmypass\n", "init")

	// Set a value
	r.mustRunWithStdin("mypass\n", "set", "KEY", "value")

	// Change to empty passphrase
	r.mustRunWithStdin("mypass\n\n\n", "passwd")

	// get should now work without passphrase
	out := r.mustRun("get", "KEY")
	if out != "value" {
		t.Fatalf("get after passwd to empty = %q, want %q", out, "value")
	}
}

func TestIntegration_InitAlreadyExists(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	// Second init should succeed (idempotent, prints message)
	_, stderr, err := r.runWithStdin("\n\n", "init")
	if err != nil {
		t.Fatalf("second init should not error: %v", err)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Fatalf("second init should mention already exists: %s", stderr)
	}
}

func TestIntegration_NoStore(t *testing.T) {
	r := newRunner(t)

	// get without init should fail
	_, stderr := r.mustFail("get", "KEY")
	if !strings.Contains(stderr, "No store found") {
		t.Fatalf("expected 'No store found' error, got: %s", stderr)
	}

	// set without init should fail
	_, stderr = r.mustFail("set", "KEY", "value")
	if !strings.Contains(stderr, "No store found") {
		t.Fatalf("expected 'No store found' error, got: %s", stderr)
	}

	// ls without init should fail
	_, stderr = r.mustFail("ls")
	if !strings.Contains(stderr, "No store found") {
		t.Fatalf("expected 'No store found' error, got: %s", stderr)
	}
}

func TestIntegration_WrongPassphrase(t *testing.T) {
	r := newRunner(t)

	// Init with passphrase
	r.mustRunWithStdin("correctpass\ncorrectpass\n", "init")
	r.mustRunWithStdin("correctpass\n", "set", "KEY", "value")

	// Get with wrong passphrase
	_, stderr, err := r.runWithStdin("wrongpass\n", "get", "KEY")
	if err == nil {
		t.Fatal("get with wrong passphrase should fail")
	}
	if !strings.Contains(stderr, "Incorrect passphrase") {
		t.Fatalf("expected passphrase error, got: %s", stderr)
	}
}

func TestIntegration_ExportMissingManifest(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	_, stderr := r.mustFail("export", "-f", filepath.Join(r.workDir, "nonexistent.yaml"))
	if !strings.Contains(stderr, "manifest not found") {
		t.Fatalf("expected 'manifest not found' error, got: %s", stderr)
	}
}

func TestIntegration_ExportInvalidFormat(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.writeFile(".secrets.yaml", `project: test
keys:
  - KEY
`)

	_, stderr := r.mustFail("export", "-f", filepath.Join(r.workDir, ".secrets.yaml"), "--format", "invalid")
	if !strings.Contains(stderr, "invalid") || !strings.Contains(strings.ToLower(stderr), "format") {
		t.Fatalf("expected format error, got: %s", stderr)
	}
}

func TestIntegration_Version(t *testing.T) {
	r := newRunner(t)
	out := r.mustRun("--version")
	if !strings.HasPrefix(out, "secrets ") {
		t.Fatalf("version output should start with 'secrets ', got: %q", out)
	}
}

func TestIntegration_ExportCustomMapFile(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "GLOBAL_TOKEN", "tok123")

	r.writeFile(".secrets.yaml", `project: test
map: custom-map.yaml
keys:
  - LOCAL_TOKEN
`)
	r.writeFile("custom-map.yaml", `LOCAL_TOKEN: GLOBAL_TOKEN
`)

	out := r.mustRun("export", "-f", filepath.Join(r.workDir, ".secrets.yaml"))
	if !strings.Contains(out, "LOCAL_TOKEN") {
		t.Fatalf("custom map export missing LOCAL_TOKEN: %s", out)
	}
	if !strings.Contains(out, "tok123") {
		t.Fatalf("custom map export has wrong value: %s", out)
	}
}

func TestIntegration_DumpEmpty(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	out := r.mustRun("dump")
	if strings.TrimSpace(out) != "" {
		t.Fatalf("dump of empty store should be empty, got: %q", out)
	}
}

func TestIntegration_ExportOrderPreserved(t *testing.T) {
	r := newRunner(t)
	r.initNoPassphrase()

	r.mustRun("set", "ZEBRA", "z")
	r.mustRun("set", "ALPHA", "a")
	r.mustRun("set", "MIKE", "m")

	r.writeFile(".secrets.yaml", `project: test
keys:
  - MIKE
  - ZEBRA
  - ALPHA
`)

	out := r.mustRun("export", "-f", filepath.Join(r.workDir, ".secrets.yaml"))
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("export returned %d lines, want 3", len(lines))
	}
	// Order should match manifest, not alphabetical
	if !strings.Contains(lines[0], "MIKE") {
		t.Fatalf("first line should contain MIKE, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "ZEBRA") {
		t.Fatalf("second line should contain ZEBRA, got: %s", lines[1])
	}
	if !strings.Contains(lines[2], "ALPHA") {
		t.Fatalf("third line should contain ALPHA, got: %s", lines[2])
	}
}
