package relay

import (
	"io"
	"net"
	"net/http"
	"time"
)

// HealthServer answers the platform's readiness probe on its own listener, so
// connections that fill the relay's 128 slots cannot fail the probe and get
// the instance restarted, ending every room (DESIGN_MULTIPLAYER §16.5.6). It
// reports no room information.
type HealthServer struct {
	listener net.Listener
	server   *http.Server
	done     chan struct{}
}

func ListenHealth(address string) (*HealthServer, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, localIOError("health listen", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "ok\n")
	})
	h := &HealthServer{listener: listener, done: make(chan struct{}), server: &http.Server{
		Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second,
		WriteTimeout: 2 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 4 << 10,
	}}
	go func() { defer close(h.done); _ = h.server.Serve(listener) }()
	return h, nil
}

func (h *HealthServer) Addr() string { return h.listener.Addr().String() }

func (h *HealthServer) Close() error {
	err := h.server.Close()
	<-h.done
	return err
}
