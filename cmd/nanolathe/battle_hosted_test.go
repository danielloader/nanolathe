package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestHostedMultiplayerLaunchOptions(t *testing.T) {
	base := []string{"--relay-address", "relay.example.test:39032", "--map", "ashap plateau"}
	for _, extra := range [][]string{nil, {"--relay-room", "ABCDEFGHJK"}, {"--relay-ca", "private.pem"}} {
		o, err := parseFlags(append(append([]string{}, base...), extra...), io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if !o.multiplayerPlaytest() || o.localMultiplayer() || !o.ignoresSavedSelection() || o.Arrival || !o.GameplaySet {
			t.Fatal("hosted launch did not select the fixed multiplayer entry")
		}
	}
	for _, args := range [][]string{
		{"--relay-address", "missing-port", "--map", "m"},
		{"--relay-address", ":39032", "--map", "m"},
		{"--relay-address", "host:0", "--map", "m"},
		{"--relay-room", "ABCDEFGHJK"},
		{"--relay-insecure-loopback"},
		{"--relay-ca", "private.pem"},
		append(append([]string{}, base...), "--local-mp-listen", "127.0.0.1:39031"),
		append(append([]string{}, base...), "--local-mp-command-delay-ms", "0"),
		append(append([]string{}, base...), "--relay-insecure-loopback"),
		append(append([]string{}, base...), "--headless"),
		append(append([]string{}, base...), "--ai-player", "all=modern"),
	} {
		if _, err := parseFlags(args, io.Discard); err == nil {
			t.Fatalf("admitted %v", args)
		}
	}
	if _, err := parseFlags([]string{"--relay-address", "127.0.0.1:39032", "--map", "m", "--relay-insecure-loopback"}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestHostedMultiplayerTrustCertificate(t *testing.T) {
	options, err := hostedDialOptions(Options{})
	if err != nil || options.TLSConfig != nil || options.InsecureLoopback {
		t.Fatal("default trust changed")
	}
	path := filepath.Join(t.TempDir(), "invalid.pem")
	if _, err := hostedDialOptions(Options{RelayCA: path}); err == nil {
		t.Fatal("missing certificate accepted")
	}
	if err := os.WriteFile(path, []byte("not a certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := hostedDialOptions(Options{RelayCA: path}); err == nil {
		t.Fatal("malformed certificate accepted")
	}
}

func TestHostedWebSocketLaunchAddress(t *testing.T) {
	for _, address := range []string{"wss://relay.example.com/relay", "wss://relay.example.com:443/relay", "wss://[::1]/relay"} {
		if _, err := parseFlags([]string{"--map=m", "--relay-address=" + address}, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	for _, address := range []string{"ws://relay.example.com/relay", "https://relay.example.com/relay", "wss://relay.example.com/", "wss://relay.example.com/relay?key=x", "wss://relay.example.com/relay?", "wss://user@relay.example.com/relay", "wss://relay.example.com:0/relay", "wss://relay.example.com:/relay", "wss://relay.example.com/relay#x", "wss://relay.example.com/relay#"} {
		if _, err := parseFlags([]string{"--map=m", "--relay-address=" + address}, io.Discard); err == nil {
			t.Fatalf("admitted %s", address)
		}
	}
	for _, address := range []string{"ws://127.0.0.1:8080/relay", "ws://[::1]:8080/relay"} {
		if _, err := parseFlags([]string{"--map=m", "--relay-address=" + address, "--relay-insecure-loopback"}, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	for _, address := range []string{"ws://localhost:8080/relay", "ws://192.0.2.1:8080/relay", "wss://127.0.0.1/relay"} {
		if _, err := parseFlags([]string{"--map=m", "--relay-address=" + address, "--relay-insecure-loopback"}, io.Discard); err == nil {
			t.Fatalf("admitted %s", address)
		}
	}
}
