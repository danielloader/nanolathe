package modfetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func previewPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	im := image.NewPaletted(image.Rect(0, 0, width, height), color.Palette{color.Black, color.White})
	im.SetColorIndex(0, 0, 1)
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func previewEntry(target string, data []byte) Entry {
	return Entry{Map: "maps/authored.ota", Preview: &Archive{URL: target, Size: int64(len(data)), SHA256: digest(data)}}
}

func TestMapPreviewManifestAdmission(t *testing.T) {
	base, _ := url.Parse(DefaultMapCatalogURL)
	for _, target := range []string{"previews/authored-1.png", releaseURL} {
		m := mapManifest(t)
		m.Maps[0].Preview = &Archive{URL: target, Size: maxPreviewBytes, SHA256: strings.Repeat("AB", 32)}
		raw, _ := json.Marshal(m)
		got, err := parseManifest(raw, base, originOf(base))
		if err != nil {
			t.Fatal(err)
		}
		resolved, _ := base.Parse(target)
		if got.Maps[0].Preview.URL != resolved.String() || got.Maps[0].Preview.SHA256 != strings.Repeat("ab", 32) {
			t.Fatalf("preview was not normalized: %+v", got.Maps[0].Preview)
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*Manifest)
	}{
		{"foreign origin", func(m *Manifest) { m.Maps[0].Preview.URL = "https://example.com/map.png" }},
		{"empty URL", func(m *Manifest) { m.Maps[0].Preview.URL = "" }},
		{"credentials", func(m *Manifest) { m.Maps[0].Preview.URL = "https://user@nanolathe.gg/map.png" }},
		{"other release owner", func(m *Manifest) {
			m.Maps[0].Preview.URL = "https://github.com/other/maps/releases/download/maps/map.png"
		}},
		{"empty size", func(m *Manifest) { m.Maps[0].Preview.Size = 0 }},
		{"too large", func(m *Manifest) { m.Maps[0].Preview.Size = maxPreviewBytes + 1 }},
		{"bad hash", func(m *Manifest) { m.Maps[0].Preview.SHA256 = "bad" }},
		{"dependency preview", func(m *Manifest) { m.Dependencies[0].Preview = m.Maps[0].Preview }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := mapManifest(t)
			m.Maps[0].Preview = &Archive{URL: "previews/map.png", Size: 1, SHA256: strings.Repeat("a", 64)}
			tc.change(&m)
			raw, _ := json.Marshal(m)
			if _, err := parseManifest(raw, base, originOf(base)); err == nil {
				t.Fatal("accepted invalid preview")
			}
		})
	}
	// Legacy mod catalogues ignored unknown preview fields of every JSON type.
	for _, value := range []string{`123`, `"unknown"`, `{"url":false}`, `[]`} {
		raw := entryJSON("mod", "1", "mod.zip", 1, strings.Repeat("a", 64))
		raw = strings.TrimSuffix(raw, "}") + `,"PREVIEW":` + value + `}`
		m, err := parseManifest([]byte(manifestJSON(raw)), base, originOf(base))
		if err != nil || m.Mods[0].Preview != nil {
			t.Fatalf("legacy preview %s was not ignored: %+v, %v", value, m, err)
		}
	}
}

func TestPreviewCacheVerifiesBytesAndWorksOffline(t *testing.T) {
	server := newCatalogServer(t)
	client := server.client(t)
	data := previewPNG(t, 5, 3)
	entry := previewEntry(server.URL+"/preview.png", data)
	server.route("/preview.png", serveBytes(data))
	got, err := client.FetchPreview(context.Background(), entry)
	if err != nil || got.Bounds() != image.Rect(0, 0, 5, 3) || color.RGBAModel.Convert(got.At(0, 0)) != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("preview: %v, %v", got, err)
	}
	server.route("/preview.png", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	// A new client also finds the persistent cache, without trying the server.
	client = &Client{CatalogURL: client.CatalogURL, CacheDir: client.CacheDir}
	if _, err := client.FetchPreview(context.Background(), entry); err != nil || len(server.seen()) != 1 {
		t.Fatalf("offline cache made a request or failed: %v", err)
	}
	cache := filepath.Join(client.CacheDir, ".previews", entry.Preview.SHA256+".png")
	corrupt := append([]byte(nil), data...)
	corrupt[len(corrupt)-1] ^= 1
	if err := os.WriteFile(cache, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FetchPreview(context.Background(), entry); err == nil || len(server.seen()) != 2 {
		t.Fatalf("same-sized corrupt cache was trusted: %v", err)
	}
	server.route("/preview.png", serveBytes(data))
	if _, err := client.FetchPreview(context.Background(), entry); err != nil {
		t.Fatalf("repairing cache: %v", err)
	}
	if got, _ := os.ReadFile(cache); !bytes.Equal(got, data) {
		t.Fatal("cache was not repaired")
	}
}

func TestPreviewRefusesUntrustedOrInvalidBytes(t *testing.T) {
	server := newCatalogServer(t)
	client := server.client(t)
	data := previewPNG(t, 1, 1)
	entry := previewEntry(server.URL+"/preview.png", data)
	server.route("/preview.png", serveBytes(data))
	for _, change := range []func(*Archive){
		func(p *Archive) { p.Size = maxPreviewBytes + 1 },
		func(p *Archive) { p.URL = "https://elsewhere.example/preview.png" },
		func(p *Archive) { p.SHA256 = "bad" },
	} {
		bad := *entry.Preview
		change(&bad)
		if _, err := client.FetchPreview(context.Background(), Entry{Preview: &bad}); err == nil {
			t.Fatal("accepted invalid preview reference")
		}
	}
	if len(server.seen()) != 0 {
		t.Fatal("invalid preview reference made a request")
	}
	badHash := *entry.Preview
	badHash.SHA256 = strings.Repeat("0", 64)
	if _, err := client.FetchPreview(context.Background(), Entry{Preview: &badHash}); err == nil {
		t.Fatal("accepted preview with wrong digest")
	}
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewGray(image.Rect(0, 0, 1, 1)), nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"oversized width", previewPNG(t, maxPreviewDimension+1, 1)},
		{"oversized height", previewPNG(t, 1, maxPreviewDimension+1)},
		{"JPEG", jpegData.Bytes()},
		{"truncated PNG", data[:len(data)-8]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server.route("/preview.png", serveBytes(tc.data))
			if _, err := client.FetchPreview(context.Background(), previewEntry(entry.Preview.URL, tc.data)); err == nil {
				t.Fatal("accepted unsupported image")
			}
		})
	}
	if got, err := client.FetchPreview(context.Background(), Entry{}); got != nil || err != nil {
		t.Fatalf("missing optional preview: %v, %v", got, err)
	}
}

func TestPreviewCancellationPreservesResume(t *testing.T) {
	server := newCatalogServer(t)
	client := server.client(t)
	data := previewPNG(t, 3, 2)
	entry := previewEntry(server.URL+"/preview.png", data)
	started := make(chan struct{})
	server.route("/preview.png", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data[:20])
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := client.FetchPreview(ctx, entry); done <- err }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled fetch: %v", err)
	}
	// Cancellation may race the first body read; seed the exact verified
	// prefix so the retry also locks the preview's hash-bound resume path.
	dst := filepath.Join(client.CacheDir, ".previews", entry.Preview.SHA256+".png")
	part := partialName(dst, Entry{Archive: *entry.Preview})
	if err := os.WriteFile(part, data[:20], 0o644); err != nil {
		t.Fatal(err)
	}
	server.route("/preview.png", serveBytes(data))
	if _, err := client.FetchPreview(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	requests := server.seen()
	if len(requests) != 2 || requests[1].Header.Get("Range") != "bytes=20-" {
		t.Fatal("preview retry did not resume its partial download")
	}
	if _, err := client.FetchPreview(ctx, entry); !errors.Is(err, context.Canceled) || len(server.seen()) != 2 {
		t.Fatalf("cancelled cached fetch: %v", err)
	}
}

func TestPreviewRedirectTrust(t *testing.T) {
	data := previewPNG(t, 2, 2)
	hosts := fakeHosts{
		"nanolathe.gg": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://assets.example/preview.png", http.StatusFound)
		}),
		"github.com": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://assets.example/preview.png", http.StatusFound)
		}),
		"assets.example": serveBytes(data),
	}
	client := &Client{CatalogURL: DefaultMapCatalogURL, HTTP: &http.Client{Transport: hosts}}
	if _, err := client.FetchPreview(context.Background(), previewEntry("https://nanolathe.gg/preview.png", data)); !errors.Is(err, ErrOriginRefused) {
		t.Fatalf("same-origin preview followed a foreign redirect: %v", err)
	}
	if _, err := client.FetchPreview(context.Background(), previewEntry(releaseURL, data)); err != nil {
		t.Fatalf("release preview redirect: %v", err)
	}
	hosts["github.com"] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://assets.example/preview.png", http.StatusFound)
	})
	if _, err := client.FetchPreview(context.Background(), previewEntry(releaseURL, data)); !errors.Is(err, ErrOriginRefused) {
		t.Fatalf("release preview followed an insecure redirect: %v", err)
	}
}

// Observe the wait without sleeps: before a queued fetch makes a request,
// claimPreview is its only reader of Done.
type waitingPreviewContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *waitingPreviewContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestPreviewReselectionWaitsForCancelledWriter(t *testing.T) {
	data := previewPNG(t, 3, 2)
	other := previewPNG(t, 4, 2)
	started, finish := make(chan struct{}), make(chan struct{})
	var finishOnce sync.Once
	unblock := func() { finishOnce.Do(func() { close(finish) }) }
	t.Cleanup(unblock)
	var requests atomic.Int32
	hosts := fakeHosts{"nanolathe.gg": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/other.png" {
			serveBytes(other)(w, r)
			return
		}
		if requests.Add(1) == 1 {
			close(started)
		}
		// Model a transport still finishing a cancelled request. A new
		// worker must not open the same part until this one returns.
		<-finish
		serveBytes(data)(w, r)
	})}
	client := &Client{CatalogURL: DefaultMapCatalogURL, CacheDir: t.TempDir(), HTTP: &http.Client{Transport: hosts}}
	entry := previewEntry("https://nanolathe.gg/preview.png", data)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	firstDone := make(chan error, 1)
	go func() { _, err := client.FetchPreview(firstCtx, entry); firstDone <- err }()
	<-started
	cancelFirst()

	// A separate Client shares the same destination and must wait too.
	secondClient := *client
	deadlineCtx, cancelDeadline := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelDeadline()
	secondCtx := &waitingPreviewContext{Context: deadlineCtx, waiting: make(chan struct{})}
	secondDone := make(chan error, 1)
	go func() { _, err := secondClient.FetchPreview(secondCtx, entry); secondDone <- err }()
	<-secondCtx.waiting
	if requests.Load() != 1 {
		t.Fatal("reselection started a second writer before cancellation finished")
	}
	thirdBase, cancelThird := context.WithCancel(deadlineCtx)
	defer cancelThird()
	thirdCtx := &waitingPreviewContext{Context: thirdBase, waiting: make(chan struct{})}
	thirdDone := make(chan error, 1)
	go func() { _, err := secondClient.FetchPreview(thirdCtx, entry); thirdDone <- err }()
	<-thirdCtx.waiting
	cancelThird()
	if err := <-thirdDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("queued cancellation: %v", err)
	}
	if _, err := secondClient.FetchPreview(deadlineCtx, previewEntry("https://nanolathe.gg/other.png", other)); err != nil {
		t.Fatalf("an unrelated preview waited on the cancelled writer: %v", err)
	}
	unblock()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first cancellation: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("reselected preview: %v", err)
	}
	if requests.Load() != 1 {
		t.Fatal("reselection did not reuse the completed verified cache")
	}
	key, _ := filepath.Abs(filepath.Join(client.CacheDir, ".previews", entry.Preview.SHA256+".png"))
	previewTransfers.Lock()
	_, retained := previewTransfers.active[key]
	previewTransfers.Unlock()
	if retained {
		t.Fatal("completed preview retained its coordination entry")
	}
}
