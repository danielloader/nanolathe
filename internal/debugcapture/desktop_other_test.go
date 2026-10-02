//go:build !windows

package debugcapture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopLocations(t *testing.T) {
	home := t.TempDir()
	config := t.TempDir()
	fallback := filepath.Join(home, "Desktop")
	for _, goos := range []string{"darwin", "linux"} {
		got, err := desktopDirectoryFor(goos, home, config)
		if err != nil || got != fallback {
			t.Fatalf("%s without config: %q, %v", goos, got, err)
		}
	}
	for _, tc := range []struct {
		name, contents, want string
	}{
		{"localized", "# desktop settings\nXDG_DOWNLOAD_DIR=\"$HOME/Downloads\"\nXDG_DESKTOP_DIR=\"$HOME/Bureau du joueur\"\n", filepath.Join(home, "Bureau du joueur")},
		{"redirected", `XDG_DESKTOP_DIR="/mounted/Player Desktop"`, "/mounted/Player Desktop"},
		{"home", `XDG_DESKTOP_DIR="$HOME/"`, home},
		{"escaped", `XDG_DESKTOP_DIR="$HOME/Desk\"top\$\\\` + "`" + `end"`, filepath.Join(home, "Desk\"top$\\`end")},
		{"relative", `XDG_DESKTOP_DIR="relative/Desktop"`, fallback},
		{"malformed", `XDG_DESKTOP_DIR="$HOME/Desktop`, fallback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(config, "user-dirs.dirs"), []byte(tc.contents), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := desktopDirectoryFor("linux", home, config)
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	if err := os.Mkdir(filepath.Join(home, ".config"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "user-dirs.dirs"), []byte(`XDG_DESKTOP_DIR="$HOME/Local Desktop"`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := desktopDirectoryFor("linux", home, "")
	if err != nil || got != filepath.Join(home, "Local Desktop") {
		t.Fatalf("default config: %q, %v", got, err)
	}
	got, err = desktopDirectoryFor("darwin", home, config)
	if err != nil || got != fallback {
		t.Fatalf("macOS used XDG config: %q, %v", got, err)
	}
}
