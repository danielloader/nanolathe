//go:build !windows

package debugcapture

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func desktopDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return desktopDirectoryFor(runtime.GOOS, home, os.Getenv("XDG_CONFIG_HOME"))
}

func desktopDirectoryFor(goos, home, config string) (string, error) {
	fallback := filepath.Join(home, "Desktop")
	if goos != "linux" {
		return fallback, nil
	}
	if !filepath.IsAbs(config) {
		config = filepath.Join(home, ".config")
	}
	data, err := os.ReadFile(filepath.Join(config, "user-dirs.dirs"))
	if errors.Is(err, os.ErrNotExist) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "XDG_DESKTOP_DIR" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
			continue
		}
		value = value[1 : len(value)-1]
		// XDG user dirs use a quoted absolute path or "$HOME/Path". Decode
		// shell escapes as data; never source the file or execute its contents.
		homeRelative := value == "$HOME" || strings.HasPrefix(value, "$HOME/")
		if homeRelative {
			value = strings.TrimPrefix(value, "$HOME")
		} else if !filepath.IsAbs(value) {
			continue
		}
		value = strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\$`, `$`, "\\`", "`").Replace(value)
		if homeRelative {
			value = home + value
		}
		return filepath.Clean(value), nil
	}
	return fallback, nil
}
