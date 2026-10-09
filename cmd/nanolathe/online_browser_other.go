//go:build !js

package main

import "github.com/nanolathe-gg/nanolathe/internal/client"

// watchOnlineBackground starts the browser page's background step
// (online_browser_js.go); native hosts have none.
func watchOnlineBackground(*gameShell, *client.Client) {}
