package pct

import (
	"fmt"
	"strings"
	"testing"
)

// mockPctRun replaces pctRun for testing, recording args and returning canned output.
type mockRunner struct {
	lastArgs []string
	output   string
	err      error
}

func (m *mockRunner) run(args ...string) (string, error) {
	m.lastArgs = args
	return m.output, m.err
}

func withMockPctRun(t *testing.T, output string, err error) *mockRunner {
	t.Helper()
	m := &mockRunner{output: output, err: err}
	orig := pctRun
	pctRun = m.run
	t.Cleanup(func() { pctRun = orig })
	return m
}

func withMockExecInCT(t *testing.T, output string, exitCode int, err error) {
	t.Helper()
	orig := pctExecInCT
	pctExecInCT = func(ctid int, command []string) (*ExecResult, error) {
		if err != nil {
			return nil, err
		}
		return &ExecResult{Output: output, ExitCode: exitCode}, nil
	}
	t.Cleanup(func() { pctExecInCT = orig })
}

// --- Exec ---

func TestExecEmptyCommand(t *testing.T) {
	_, err := Exec(100, []string{})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Errorf("error = %v, want 'empty command'", err)
	}
}

func TestExecSuccess(t *testing.T) {
	withMockExecInCT(t, "hello world\n", 0, nil)
	result, err := Exec(100, []string{"echo", "hello", "world"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Output != "hello world\n" {
		t.Errorf("Output = %q, want %q", result.Output, "hello world\n")
	}
}

func TestExecNonZeroExit(t *testing.T) {
	withMockExecInCT(t, "not found\n", 1, nil)
	result, err := Exec(100, []string{"cat", "/no/such/file"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
}

func TestExecError(t *testing.T) {
	withMockExecInCT(t, "", 0, fmt.Errorf("connection refused"))
	_, err := Exec(100, []string{"ls"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- BuildExecScriptCommand ---

func TestBuildExecScriptNoEnv(t *testing.T) {
	cmd := BuildExecScriptCommand("/opt/install.sh", nil)
	if len(cmd) != 2 {
		t.Fatalf("len = %d, want 2", len(cmd))
	}
	if cmd[0] != "/bin/bash" || cmd[1] != "/opt/install.sh" {
		t.Errorf("cmd = %v, want [/bin/bash /opt/install.sh]", cmd)
	}
}

func TestBuildExecScriptEmptyEnv(t *testing.T) {
	cmd := BuildExecScriptCommand("/opt/install.sh", map[string]string{})
	if len(cmd) != 2 {
		t.Fatalf("len = %d, want 2", len(cmd))
	}
	if cmd[0] != "/bin/bash" {
		t.Errorf("cmd[0] = %q, want %q", cmd[0], "/bin/bash")
	}
}

func TestBuildExecScriptWithEnv(t *testing.T) {
	cmd := BuildExecScriptCommand("/opt/install.sh", map[string]string{
		"DB_PASS": "secret123",
	})
	if cmd[0] != "env" {
		t.Errorf("cmd[0] = %q, want %q", cmd[0], "env")
	}
	// Should contain the env var somewhere
	found := false
	for _, arg := range cmd {
		if arg == "DB_PASS=secret123" {
			found = true
		}
	}
	if !found {
		t.Errorf("cmd %v should contain DB_PASS=secret123", cmd)
	}
	// Last two should be /bin/bash and the script
	if cmd[len(cmd)-2] != "/bin/bash" || cmd[len(cmd)-1] != "/opt/install.sh" {
		t.Errorf("last two args = %v, want [/bin/bash /opt/install.sh]", cmd[len(cmd)-2:])
	}
}

// --- ParseIPOutput ---

func TestParseIPSingle(t *testing.T) {
	got := ParseIPOutput("192.168.1.100 \n")
	if got != "192.168.1.100" {
		t.Errorf("got %q, want %q", got, "192.168.1.100")
	}
}

func TestParseIPMultiple(t *testing.T) {
	got := ParseIPOutput("192.168.1.100 10.0.0.1 ")
	if got != "192.168.1.100" {
		t.Errorf("got %q, want %q", got, "192.168.1.100")
	}
}

func TestParseIPEmpty(t *testing.T) {
	got := ParseIPOutput("")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestGetIPWithMock(t *testing.T) {
	withMockExecInCT(t, "10.0.0.5 \n", 0, nil)
	ip, err := GetIP(100)
	if err != nil {
		t.Fatalf("GetIP: %v", err)
	}
	if ip != "10.0.0.5" {
		t.Errorf("ip = %q, want %q", ip, "10.0.0.5")
	}
}

func TestGetIPExecFailure(t *testing.T) {
	withMockExecInCT(t, "", 1, nil)
	_, err := GetIP(100)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}

// --- Push with mock ---

func TestPushSuccess(t *testing.T) {
	m := withMockPctRun(t, "", nil)
	err := Push(100, "/tmp/script.sh", "/opt/install.sh", "0755")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if m.lastArgs[0] != "push" {
		t.Errorf("args[0] = %q, want %q", m.lastArgs[0], "push")
	}
	assertContains(t, m.lastArgs, "/tmp/script.sh")
	assertContains(t, m.lastArgs, "/opt/install.sh")
	assertContainsPair(t, m.lastArgs, "--perms", "0755")
}

func TestPushNoPerms(t *testing.T) {
	m := withMockPctRun(t, "", nil)
	Push(100, "/tmp/a", "/opt/a", "")
	assertNotContains(t, m.lastArgs, "--perms")
}

// --- Set (device passthrough) ---

func TestSetSuccess(t *testing.T) {
	m := withMockPctRun(t, "", nil)
	err := Set(100, "-dev0", "/dev/dri/renderD128,gid=44,mode=0666")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if m.lastArgs[0] != "set" {
		t.Errorf("args[0] = %q, want %q", m.lastArgs[0], "set")
	}
	if m.lastArgs[1] != "100" {
		t.Errorf("args[1] = %q, want %q", m.lastArgs[1], "100")
	}
	assertContains(t, m.lastArgs, "-dev0")
	assertContains(t, m.lastArgs, "/dev/dri/renderD128,gid=44,mode=0666")
}

func TestSetError(t *testing.T) {
	withMockPctRun(t, "error: permission denied", fmt.Errorf("exit status 1"))
	err := Set(100, "-dev0", "/dev/nvidia0")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pct set") {
		t.Errorf("error = %v, want 'pct set' in message", err)
	}
}

// --- helpers ---

func assertContains(t *testing.T, args []string, want string) {
	t.Helper()
	for _, a := range args {
		if a == want {
			return
		}
	}
	t.Errorf("args %v does not contain %q", args, want)
}

func assertNotContains(t *testing.T, args []string, unwanted string) {
	t.Helper()
	for _, a := range args {
		if a == unwanted {
			t.Errorf("args %v should not contain %q", args, unwanted)
			return
		}
	}
}

func assertContainsPair(t *testing.T, args []string, key, value string) {
	t.Helper()
	for i, a := range args {
		if a == key && i+1 < len(args) && args[i+1] == value {
			return
		}
	}
	t.Errorf("args %v does not contain pair [%q %q]", args, key, value)
}
