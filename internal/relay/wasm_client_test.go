package relay

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// wasmTerminalTick ends the browser seats' match: two checksum ticks.
const wasmTerminalTick = 60

// wasmNodeMajor is the first Node release with a global WebSocket, which the
// browser build's relay client uses under Node as it does in a page.
const wasmNodeMajor = 22

// TestWasmClientPlaysHostedMatch runs the browser build's relay client — this
// package's js/wasm test binary under Node — against a native relay on
// loopback (DESIGN_MULTIPLAYER §16.5.1). TestWasmSeats, in that binary,
// plays both seats of a room through the browser's WebSocket. It is opt-in
// because it builds a second binary; tools/check-retail and the CI browser
// job run it with NANOLATHE_WASM_RELAY=1.
func TestWasmClientPlaysHostedMatch(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("the harness runs natively")
	}
	if os.Getenv("NANOLATHE_WASM_RELAY") == "" {
		t.Skip("set NANOLATHE_WASM_RELAY=1 to run the js/wasm relay client under Node")
	}
	if err := wasmNode(); err != "" {
		t.Skip("SKIPPED: " + err)
	}
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	wasm := filepath.Join(t.TempDir(), "relay.wasm")
	build := exec.Command("go", "test", "-c", "-o", wasm, ".")
	build.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the js/wasm test binary: %v\n%s", err, out)
	}

	timeouts := hostedDefaultTimeouts
	// Node must answer the relay's pings, and the seats see their reports.
	timeouts.ping, timeouts.report = 100*time.Millisecond, 100*time.Millisecond
	s, err := listenHostedWebSocket("127.0.0.1:0", HostedConfig{InsecureLoopback: true}, false, timeouts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	bare := listenBareUpgrade(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	runner := filepath.Join(strings.TrimSpace(string(goroot)), "lib", "wasm", "go_js_wasm_exec")
	run := exec.CommandContext(ctx, runner, wasm, "-test.run=^TestWasmSeats$", "-test.count=1", "-test.v")
	// Go's wasm port has one thread; a host GOMAXPROCS budget would stop it.
	run.Env = append(os.Environ(), "GOMAXPROCS=1", "NANOLATHE_WASM_RELAY_URL=ws://"+s.Addr()+"/relay",
		"NANOLATHE_WASM_BARE_URL=ws://"+bare+"/relay")
	var output bytes.Buffer
	run.Stdout, run.Stderr = &output, &output
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- run.Wait() }()
	// The relay measures a seat's round trip only from pongs to its pings.
	answered := false
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for waiting := true; waiting; {
		select {
		case err = <-done:
			waiting = false
		case <-poll.C:
			for _, room := range s.Status().Rooms {
				for _, seat := range room.Seats {
					answered = answered || seat.RTTMillis > 0
				}
			}
		}
	}
	if err != nil || !strings.Contains(output.String(), "--- PASS: TestWasmSeats") {
		t.Fatalf("js/wasm seats: %v\n%s", err, output.String())
	}
	t.Logf("js/wasm seats:\n%s", output.String())
	if !answered {
		t.Fatal("the browser WebSocket never answered the relay's pings")
	}
	awaitHostedCapacity(t, s, 0, 0)
	if st := s.Status(); len(st.Recent) != 1 || st.Recent[0].Outcome != finishCompleted || st.Recent[0].Players != 2 || st.Recent[0].Ticks < wasmTerminalTick {
		t.Fatalf("the browser room did not complete: %+v", st.Recent)
	}
}

// wasmNode reports why Node cannot run the browser client, or "".
func wasmNode() string {
	out, err := exec.Command("node", "-p", "process.versions.node").Output()
	if err != nil {
		return "Node is not on PATH"
	}
	version := strings.TrimSpace(string(out))
	major, err := strconv.Atoi(strings.SplitN(version, ".", 2)[0])
	if err != nil || major < wasmNodeMajor {
		return "Node " + version + " has no global WebSocket; " + strconv.Itoa(wasmNodeMajor) + " or later is needed"
	}
	return ""
}

// listenBareUpgrade serves WebSocket upgrades that name no subprotocol, which
// a relay client must refuse.
func listenBareUpgrade(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				header, err := readWebSocketHeader(bufio.NewReader(conn))
				if err != nil {
					return
				}
				req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(header)))
				if err != nil {
					return
				}
				fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", websocketAccept(req.Header.Get("Sec-WebSocket-Key")))
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()
	return l.Addr().String()
}
