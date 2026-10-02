package debugcapture

import "golang.org/x/sys/windows"

func desktopDirectory() (string, error) {
	// Ask the shell so redirected Desktops (including OneDrive) are honored.
	return windows.KnownFolderPath(windows.FOLDERID_Desktop, windows.KF_FLAG_DEFAULT)
}
