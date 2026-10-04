//go:build js && wasm

package benchlock

import "os"

// js/wasm cannot join the native host's advisory benchmark file lock. The
// browser host serializes runtimes at one origin with Web Locks; that lease
// does not coordinate benchmarks in other origins or native processes
// (DESIGN_BROWSER_HOST §4). The launcher exposes only its stress diagnostic.
func lock(_ *os.File, _ bool) (bool, error) { return true, nil }
