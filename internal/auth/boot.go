//go:build darwin

package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

var bootSessionTempDir = os.TempDir
var bootSessionUID = os.Getuid
var bootTimeValue = currentBootTime

func isBootSessionValid() bool {
	tokenPath, err := bootSessionTokenPath()
	if err != nil {
		return false
	}

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return false
	}

	stored := strings.TrimSpace(string(data))
	if stored == "" {
		return false
	}

	current, err := bootTimeValue()
	if err != nil {
		return false
	}
	if stored == current {
		return true
	}

	if err := os.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		return false
	}
	return false
}

func writeBootSessionToken() error {
	tokenPath, err := bootSessionTokenPath()
	if err != nil {
		return err
	}

	bootTime, err := bootTimeValue()
	if err != nil {
		return err
	}

	if err := os.WriteFile(tokenPath, []byte(bootTime+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tokenPath, 0o600); err != nil {
		return err
	}
	return nil
}

func bootSessionTokenPath() (string, error) {
	tmpDir := bootSessionTempDir()
	if tmpDir == "" {
		return "", fmt.Errorf("boot session temp dir is empty")
	}
	return filepath.Join(tmpDir, fmt.Sprintf("kc-session-%d", bootSessionUID())), nil
}

func currentBootTime() (string, error) {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return "", fmt.Errorf("read boot time: %w", err)
	}
	return strconv.FormatInt(tv.Sec, 10), nil
}
