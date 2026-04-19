package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type VMMessage struct {
	Type     string `json:"type"`
	Msg      string `json:"msg,omitempty"`
	Script   string `json:"script,omitempty"`
	OK       bool   `json:"ok,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Output   string `json:"output,omitempty"`
}

func RunSandbox(ctx context.Context) (results []VMMessage, retErr error) {
	defer func() {
		if err := clearSharedScriptsDir(); err != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v; cleanup failed: %w", retErr, err)
			} else {
				retErr = fmt.Errorf("cleanup failed: %w", err)
			}
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"qemu-system-aarch64",
		"-machine", "virt,accel=hvf",
		"-cpu", "host",
		"-m", "1024",
		"-smp", "2",
		"-drive", "file=../data/sandbox.qcow2,if=virtio",
		"-bios", "/opt/homebrew/share/qemu/edk2-aarch64-code.fd",
		"-fsdev", "local,id=fsdev0,path=../specialists/shared-scripts,security_model=none,readonly=on",
		"-device", "virtio-9p-pci,fsdev=fsdev0,mount_tag=share",
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

	var bootDone bool

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	for {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("sandbox timed out: %w", ctx.Err())

		case err := <-done:
			if err != nil {
				return nil, fmt.Errorf("qemu exited with error: %w", err)
			}
			return results, nil

		case line := <-lines:
			if line == "" {
				continue
			}

			// Ignore the noisy firmware/kernel boot output until we hit JSON
			if !strings.HasPrefix(line, "{") {
				continue
			}

			var msg VMMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}

			bootDone = true

			switch msg.Type {
			case "vm_status":
				if msg.Msg == "powering off" {
					// Keep waiting for process exit
				}
			case "result":
				results = append(results, msg)
			case "vm_error":
				return nil, fmt.Errorf("vm error: %s", msg.Msg)
			}

			_ = bootDone
		}
	}
}

func clearSharedScriptsDir() error {
	const sharedScriptsDir = "../specialists/shared-scripts"

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

	// Increase max line size in case script output gets larger
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		out <- scanner.Text()
	}
}
