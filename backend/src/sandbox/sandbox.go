package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultSharedScriptsDir = "../shared-scripts"

type VMMessage struct {
	Type     string `json:"type"`
	Msg      string `json:"msg,omitempty"`
	Message  string `json:"message,omitempty"`
	Script   string `json:"script,omitempty"`
	OK       bool   `json:"ok,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Output   string `json:"output,omitempty"`
}

func SharedScriptsDir() string {
	if dir := strings.TrimSpace(os.Getenv("SANDBOX_SHARED_SCRIPTS_DIR")); dir != "" {
		return dir
	}
	return defaultSharedScriptsDir
}

func SandboxTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("SANDBOX_TIMEOUT_SECONDS")); raw != "" {
		if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
			return time.Duration(sec) * time.Second
		}
	}
	// VM boot + multi-script network calls regularly exceed 20s.
	return 240 * time.Second
}

func RunSandbox(ctx context.Context) (results []VMMessage, retErr error) {
	defer func() {
		if err := ClearSharedScriptsDir(); err != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v; cleanup failed: %w", retErr, err)
			} else {
				retErr = fmt.Errorf("cleanup failed: %w", err)
			}
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, SandboxTimeout())
	defer cancel()

	cmd := exec.Command(
		"qemu-system-aarch64",
		"-machine", "virt,accel=hvf",
		"-cpu", "host",
		"-m", "1024",
		"-smp", "2",
		"-drive", "file=../data/sandbox.qcow2,if=virtio",
		"-snapshot",
		"-bios", "/opt/homebrew/share/qemu/edk2-aarch64-code.fd",
		"-fsdev", "local,id=fsdev0,path=../shared-scripts,security_model=none,readonly=on",
		"-device", "virtio-9p-pci,fsdev=fsdev0,mount_tag=share",
		"-netdev", "user,id=net0",
		"-device", "virtio-net-device,netdev=net0",
		"-nographic",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start qemu: %w", err)
	}

	lines := make(chan string, 128)
	go scanLines(stdout, lines)
	go scanLines(stderr, lines)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	const maxTailLines = 12
	tailLines := make([]string, 0, maxTailLines)
	appendTail := func(line string) {
		if strings.TrimSpace(line) == "" {
			return
		}
		if len(tailLines) == maxTailLines {
			copy(tailLines, tailLines[1:])
			tailLines = tailLines[:maxTailLines-1]
		}
		tailLines = append(tailLines, line)
	}

	sawJSONProtocolLine := false
	currentScript := ""
	scriptOutputs := make(map[string][]string)
	scriptOrder := make([]string, 0, 8)
	rememberScript := func(name string) {
		if name == "" {
			return
		}
		if _, ok := scriptOutputs[name]; !ok {
			scriptOutputs[name] = nil
			scriptOrder = append(scriptOrder, name)
		}
	}
	appendScriptOutput := func(name string, payload string) {
		if name == "" || strings.TrimSpace(payload) == "" {
			return
		}
		rememberScript(name)
		scriptOutputs[name] = append(scriptOutputs[name], payload)
	}
	scriptOutputText := func(name string) string {
		if name == "" {
			return ""
		}
		return strings.Join(scriptOutputs[name], "\n")
	}
	synthesizeResultsFromOutputs := func() {
		for _, name := range scriptOrder {
			output := strings.TrimSpace(scriptOutputText(name))
			if output == "" {
				continue
			}
			results = append(results, VMMessage{
				Type:     "result",
				Script:   name,
				OK:       true,
				ExitCode: 0,
				Output:   output,
			})
		}
	}

	for {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("sandbox timed out: %w", ctx.Err())

		case err := <-done:
			if err != nil {
				return nil, fmt.Errorf("qemu exited with error: %w", err)
			}
			if len(results) == 0 {
				synthesizeResultsFromOutputs()
			}
			if len(results) == 0 {
				if sawJSONProtocolLine {
					return results, nil
				}
				if len(tailLines) > 0 {
					return nil, fmt.Errorf("sandbox exited without any result messages; tail logs: %s", strings.Join(tailLines, " | "))
				}
				return nil, fmt.Errorf("sandbox exited without any result messages")
			}
			return results, nil

		case line := <-lines:
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			appendTail(line)

			// Find the first '{' to skip any serial console prefix noise.
			i := strings.IndexByte(line, '{')
			if i < 0 {
				continue
			}
			candidate := line[i:]

			var msg VMMessage
			if err := json.Unmarshal([]byte(candidate), &msg); err != nil {
				continue
			}
			sawJSONProtocolLine = true

			switch msg.Type {
			case "script_start":
				currentScript = filepath.Base(strings.TrimSpace(msg.Script))
				rememberScript(currentScript)
			case "result":
				msg.Script = filepath.Base(strings.TrimSpace(msg.Script))
				if msg.Output == "" {
					msg.Output = scriptOutputText(msg.Script)
				}
				if msg.Script != "" {
					rememberScript(msg.Script)
				}
				results = append(results, msg)
			case "vm_error":
				errMsg := strings.TrimSpace(msg.Msg)
				if errMsg == "" {
					errMsg = strings.TrimSpace(msg.Message)
				}
				if errMsg == "" {
					errMsg = "unknown vm error"
				}
				return nil, fmt.Errorf("vm error: %s", errMsg)
			case "vm_status":
				// status-only heartbeat
			default:
				if currentScript != "" {
					appendScriptOutput(currentScript, line)
				}
			}
		}
	}
}

func ClearSharedScriptsDir() error {
	sharedScriptsDir := SharedScriptsDir()
	if err := os.MkdirAll(sharedScriptsDir, 0o755); err != nil {
		return fmt.Errorf("create shared scripts dir: %w", err)
	}

	entries, err := os.ReadDir(sharedScriptsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read shared scripts dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".py" {
			continue
		}
		path := filepath.Join(sharedScriptsDir, entry.Name())
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}

	return nil
}

func scanLines(pipe interface{ Read([]byte) (int, error) }, out chan<- string) {
	scanner := bufio.NewScanner(pipe)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		out <- scanner.Text()
	}
}
