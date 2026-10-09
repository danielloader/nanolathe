package relay

import (
	"io"
	"net/http"
	"testing"
)

func TestHealthListenerAnswersOnlyHealth(t *testing.T) {
	h, err := ListenHealth("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, tc := range []struct {
		path string
		want int
	}{{"/healthz", http.StatusOK}, {"/relay", http.StatusNotFound}} {
		response, err := http.Get("http://" + h.Addr() + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != tc.want || (tc.want == http.StatusOK && string(body) != "ok\n") {
			t.Fatalf("%s: %d %q", tc.path, response.StatusCode, body)
		}
	}
}
