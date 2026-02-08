// Package pct provides safe wrappers for Proxmox pct commands that have
// no REST API equivalent: exec, push, and IP discovery.
// All commands use exec.Command with explicit argv — no shell strings.
package pct

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	pctBin     = "/usr/sbin/pct"
	nsenterBin = "/usr/bin/nsenter"
)

// nsenterArgs escapes into PID 1's mount namespace so that child processes
// (pct) see the real host filesystem instead of the
// ProtectSystem=strict read-only overlay applied by systemd.
var nsenterArgs = []string{"--mount=/proc/1/ns/mnt", "--"}

// SudoNsenterCmd builds an exec.Cmd that runs:
//
//	sudo nsenter --mount=/proc/1/ns/mnt -- <bin> <args...>
//
// This escapes the systemd mount namespace only for the specific command.
func SudoNsenterCmd(bin string, args ...string) *exec.Cmd {
	cmdArgs := make([]string, 0, 3+len(nsenterArgs)+len(args))
	cmdArgs = append(cmdArgs, nsenterBin)
	cmdArgs = append(cmdArgs, nsenterArgs...)
	cmdArgs = append(cmdArgs, bin)
	cmdArgs = append(cmdArgs, args...)
	return exec.Command("sudo", cmdArgs...)
}

// Command execution hooks — override in tests to mock system commands.
var (
	// pctRun executes a pct subcommand via sudo, returns trimmed output.
	pctRun = func(args ...string) (string, error) {
		cmd := SudoNsenterCmd(pctBin, args...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	// pctExecInCT runs a command inside a container via sudo pct exec.
	pctExecInCT = func(ctid int, command []string) (*ExecResult, error) {
		args := append([]string{"exec", strconv.Itoa(ctid), "--"}, command...)
		cmd := SudoNsenterCmd(pctBin, args...)
		out, err := cmd.CombinedOutput()

		result := &ExecResult{
			Output:   string(out),
			ExitCode: 0,
		}

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			} else {
				return nil, fmt.Errorf("pct exec %d: %w", ctid, err)
			}
		}

		return result, nil
	}
)

// ExecResult holds the output of a pct exec command.
type ExecResult struct {
	Output   string
	ExitCode int
}

// Exec runs a command inside a container via pct exec.
func Exec(ctid int, command []string) (*ExecResult, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return pctExecInCT(ctid, command)
}

// ExecStream runs a command inside a container and calls onLine for each
// line of output as it arrives, enabling real-time log streaming.
func ExecStream(ctid int, command []string, onLine func(line string)) (*ExecResult, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	args := append([]string{"exec", strconv.Itoa(ctid), "--"}, command...)
	cmd := SudoNsenterCmd(pctBin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pct exec stream %d: stdout pipe: %w", ctid, err)
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("pct exec stream %d: start: %w", ctid, err)
	}

	scanner := bufio.NewScanner(stdout)
	var output strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		output.WriteString(line)
		output.WriteByte('\n')
		if onLine != nil {
			onLine(line)
		}
	}

	result := &ExecResult{
		Output:   output.String(),
		ExitCode: 0,
	}

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("pct exec stream %d: %w", ctid, err)
		}
	}

	return result, nil
}

// BuildExecScriptCommand builds the command slice for running a script with env vars.
func BuildExecScriptCommand(scriptPath string, env map[string]string) []string {
	envParts := make([]string, 0, len(env))
	for k, v := range env {
		envParts = append(envParts, fmt.Sprintf("%s=%s", k, v))
	}

	command := []string{"/bin/bash", scriptPath}
	if len(envParts) > 0 {
		command = append([]string{"env"}, append(envParts, command...)...)
	}
	return command
}

// ExecScript runs a shell script inside the container.
func ExecScript(ctid int, scriptPath string, env map[string]string) (*ExecResult, error) {
	command := BuildExecScriptCommand(scriptPath, env)
	return Exec(ctid, command)
}

// Push copies a file from the host into the container.
func Push(ctid int, src, dst string, perms string) error {
	args := []string{"push", strconv.Itoa(ctid), src, dst}
	if perms != "" {
		args = append(args, "--perms", perms)
	}
	out, err := pctRun(args...)
	if err != nil {
		return fmt.Errorf("pct push %d %s %s: %s: %w", ctid, src, dst, out, err)
	}
	return nil
}

// GetIP attempts to get the IP address of a running container.
func GetIP(ctid int) (string, error) {
	result, err := Exec(ctid, []string{"hostname", "-I"})
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("hostname -I exited with %d", result.ExitCode)
	}
	return ParseIPOutput(result.Output), nil
}

// ParseIPOutput extracts the first IP from "hostname -I" output.
func ParseIPOutput(output string) string {
	ip := strings.TrimSpace(output)
	parts := strings.Fields(ip)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}
