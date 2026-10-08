//go:build darwin

package metalrender

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/platform/mtl"
)

// The renderer must start on the process main thread, as the game's does.
// Tests run on worker goroutines, so TestMain, which keeps the main
// goroutine, does the device work and the test reports its result.
func init() { runtime.LockOSThread() }

var deviceResult error
var deviceRan bool

func TestMain(m *testing.M) {
	if os.Getenv("NANOLATHE_METAL_DEVICE_TEST") == "1" {
		deviceRan = true
		deviceResult = checkDevicePipelines()
	}
	os.Exit(m.Run())
}

// TestDevicePipelines builds the offscreen renderer on the real device. Metal
// compiles the shader library when the game starts, so a shader error, a
// missing entry point or a pipeline whose attachments disagree with its
// fragment outputs shows up only there; this finds it before a player does.
// It is opt-in because ordinary tests must not require a graphics device;
// tools/check-retail runs it on macOS.
func TestDevicePipelines(t *testing.T) {
	if !deviceRan {
		t.Skip("set NANOLATHE_METAL_DEVICE_TEST=1 to build the Metal renderer on this Mac's device")
	}
	if deviceResult != nil {
		t.Fatal(deviceResult)
	}
}

var entryPoint = regexp.MustCompile(`(?m)\b(?:kernel|vertex|fragment)\s+[\w:<>, ]+?\s+(\w+)\s*\(`)

func checkDevicePipelines() error {
	r, err := newGoRenderer(64, 64, true, false, true, true, 16, librarySource())
	if err != nil {
		return err
	}
	defer r.destroy()
	names := entryPoint.FindAllStringSubmatch(librarySource(), -1)
	if len(names) == 0 {
		return fmt.Errorf("no entry points found in the shader library source")
	}
	for _, name := range names {
		fn := mtl.Function(r.lib, name[1])
		if fn == 0 {
			return fmt.Errorf("shader library has no function %s", name[1])
		}
		mtl.Release(fn)
	}
	return nil
}
