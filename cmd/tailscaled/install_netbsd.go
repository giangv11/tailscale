// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

//go:build netbsd

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func init() {
	installSystemDaemon = installSystemDaemonNetBSD
	uninstallSystemDaemon = uninstallSystemDaemonNetBSD
}

const (
	rcScriptPath = "/etc/rc.d/tailscaled"
	targetBin    = "/usr/sbin/tailscaled"
	rcScriptSrc  = `#!/bin/sh
# $NetBSD: tailscaled.rc.d,v 1.0 2024/01/01 00:00:00 tailscale Exp $
#
# PROVIDE: tailscaled
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="tailscaled"
rcvar=${name}
command="/usr/sbin/tailscaled"
command_args="--state=/var/lib/tailscale/tailscaled.state --socket=/var/run/tailscale/tailscaled.sock"
pidfile="/var/run/tailscale/${name}.pid"
start_precmd="tailscaled_prestart"
stop_postcmd="tailscaled_poststop"

tailscaled_prestart()
{
	# Create required directories (must be done before rc.subr checks)
	mkdir -p /var/lib/tailscale
	mkdir -p /var/run/tailscale
	chmod 700 /var/lib/tailscale
	chmod 755 /var/run/tailscale
	
	# Clean up any existing state
	${command} --cleanup 2>/dev/null || true
}

tailscaled_poststop()
{
	# Clean up on stop
	${command} --cleanup 2>/dev/null || true
}

load_rc_config $name
run_rc_command "$1"
`
)

func uninstallSystemDaemonNetBSD(args []string) (ret error) {
	if len(args) > 0 {
		return errors.New("uninstall subcommand takes no arguments")
	}

	// Stop the service if running
	if out, err := exec.Command("/etc/rc.d/tailscaled", "stop").CombinedOutput(); err != nil {
		// Ignore errors if service is not running
		_ = out
	}

	// Disable the service by removing from rc.conf
	rcConfPath := "/etc/rc.conf"
	if err := removeFromRcConf(rcConfPath, "tailscaled"); err != nil {
		if !os.IsNotExist(err) {
			ret = fmt.Errorf("failed to disable service in %s: %w", rcConfPath, err)
		}
	}

	// Remove the rc.d script
	if err := os.Remove(rcScriptPath); err != nil && !os.IsNotExist(err) {
		if ret == nil {
			ret = fmt.Errorf("failed to remove %s: %w", rcScriptPath, err)
		}
	}

	return ret
}

func installSystemDaemonNetBSD(args []string) (err error) {
	if len(args) > 0 {
		return errors.New("install subcommand takes no arguments")
	}
	defer func() {
		if err != nil && os.Getuid() != 0 {
			err = fmt.Errorf("%w; try running tailscaled with sudo", err)
		}
	}()

	// Best effort uninstall first
	uninstallSystemDaemonNetBSD(nil)

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find our own executable path: %w", err)
	}

	// Copy binary to target location if needed
	same, err := sameFile(exe, targetBin)
	if err != nil {
		return err
	}

	if !same {
		if err := copyBinary(exe, targetBin); err != nil {
			return err
		}
	}

	// Create required directories before installing the script
	if err := os.MkdirAll("/var/lib/tailscale", 0700); err != nil {
		return fmt.Errorf("failed to create /var/lib/tailscale: %w", err)
	}
	if err := os.MkdirAll("/var/run/tailscale", 0755); err != nil {
		return fmt.Errorf("failed to create /var/run/tailscale: %w", err)
	}

	// Write the rc.d script
	if err := os.WriteFile(rcScriptPath, []byte(rcScriptSrc), 0755); err != nil {
		return fmt.Errorf("failed to write %s: %w", rcScriptPath, err)
	}

	// Enable the service to start on boot by adding to rc.conf
	rcConfPath := "/etc/rc.conf"
	if err := addToRcConf(rcConfPath, "tailscaled=YES"); err != nil {
		return fmt.Errorf("failed to enable service in %s: %w", rcConfPath, err)
	}

	// Start the service
	if out, err := exec.Command("/etc/rc.d/tailscaled", "start").CombinedOutput(); err != nil {
		return fmt.Errorf("error running /etc/rc.d/tailscaled start: %v, %s", err, out)
	}

	return nil
}

// copyBinary copies binary file `src` into `dst`.
func copyBinary(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	tmpBin := dst + ".tmp"
	f, err := os.Create(tmpBin)
	if err != nil {
		return err
	}
	defer f.Close()

	srcf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcf.Close()

	_, err = io.Copy(f, srcf)
	if err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tmpBin, 0755); err != nil {
		return err
	}

	if err := os.Rename(tmpBin, dst); err != nil {
		return err
	}

	return nil
}

// sameFile checks if two paths refer to the same file.
func sameFile(path1, path2 string) (bool, error) {
	fi1, err := os.Stat(path1)
	if err != nil {
		return false, err
	}
	fi2, err := os.Stat(path2)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return os.SameFile(fi1, fi2), nil
}

// addToRcConf adds a line to /etc/rc.conf if it doesn't already exist
func addToRcConf(rcConfPath, line string) error {
	data, err := os.ReadFile(rcConfPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if line already exists
	content := string(data)
	lines := strings.Split(content, "\n")
	for _, l := range lines {
		if strings.TrimSpace(l) == line {
			return nil // Already enabled
		}
	}

	// Append the line
	newContent := content
	if len(newContent) > 0 && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += line + "\n"

	return os.WriteFile(rcConfPath, []byte(newContent), 0644)
}

// removeFromRcConf removes a line from /etc/rc.conf
func removeFromRcConf(rcConfPath, serviceName string) error {
	data, err := os.ReadFile(rcConfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, nothing to remove
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	var newLines []string
	prefix := serviceName + "="
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, prefix) {
			newLines = append(newLines, line)
		}
	}

	// Preserve the original file ending (newline or not)
	content := strings.Join(newLines, "\n")
	if len(data) > 0 && data[len(data)-1] == '\n' && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	return os.WriteFile(rcConfPath, []byte(content), 0644)
}
