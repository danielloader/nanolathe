package relay

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1" // RFC 6455's handshake digest, not an authentication primitive.
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	websocketProtocol    = "nanolathe-relay-v1"
	websocketHeaderLimit = 8 << 10
	// Pings keep idle proxied connections open and measure each seat's round
	// trip for the status page.
	websocketPingInterval = 5 * time.Second
)

// ListenHostedWebSocket serves the same rooms over RFC 6455 binary streams.
// behindTLSProxy explicitly trusts a TLS terminator on the hosting network;
// standalone listeners require native TLS or numeric-loopback test mode.
func ListenHostedWebSocket(address string, config HostedConfig, behindTLSProxy bool) (*HostedServer, error) {
	return listenHostedWebSocket(address, config, behindTLSProxy, hostedDefaultTimeouts)
}

func listenHostedWebSocket(address string, config HostedConfig, behindTLSProxy bool, timeouts hostedTimeouts) (*HostedServer, error) {
	if config.TLSConfig != nil {
		config.TLSConfig = config.TLSConfig.Clone()
		config.TLSConfig.NextProtos = []string{"http/1.1"}
	}
	return listenHostedTransport(address, config, timeouts, true, behindTLSProxy)
}

func websocketAccept(key string) string {
	digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(digest[:])
}

func websocketToken(h http.Header, field, token string) bool {
	for _, value := range h.Values(field) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// Read only the bounded header block. The original buffered reader retains
// pipelined WebSocket bytes, and HTTP body framing is never allowed to consume
// them. Both sides share this limit (DESIGN_MULTIPLAYER §16.5.6).
func readWebSocketHeader(r *bufio.Reader) ([]byte, error) {
	var header []byte
	for {
		line, err := r.ReadSlice('\n')
		if len(line) > websocketHeaderLimit-len(header) {
			return nil, hostedError("WebSocket HTTP headers", "at most 8192 bytes")
		}
		header = append(header, line...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return nil, localIOError("WebSocket HTTP headers", err)
		}
		if bytes.HasSuffix(header, []byte("\r\n\r\n")) {
			return header, nil
		}
	}
}

// page answers a GET for a status path, or reports that path is not one.
type statusPageFunc func(path string) (contentType string, body []byte, ok bool)

func acceptHostedWebSocket(conn net.Conn, timeout time.Duration, deadline time.Time, page statusPageFunc) (*websocketConn, error) {
	r := bufio.NewReader(conn)
	header, err := readWebSocketHeader(r)
	if err != nil {
		return nil, err
	}
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(header)))
	if err != nil {
		return nil, hostedError("WebSocket HTTP request", "a valid HTTP/1.1 request")
	}
	defer req.Body.Close()
	if err := conn.SetWriteDeadline(minDeadline(deadline, time.Now().Add(timeout))); err != nil {
		return nil, err
	}
	validGet := req.Method == http.MethodGet && req.ProtoMajor == 1 && req.ProtoMinor == 1 && req.Host != "" && req.ContentLength == 0 && len(req.TransferEncoding) == 0
	if validGet && req.RequestURI == "/healthz" {
		_, err = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 3\r\nConnection: close\r\n\r\nok\n")
		if err != nil {
			return nil, localIOError("WebSocket health response", err)
		}
		return nil, io.EOF // The owner closes this one-request health connection.
	}
	if validGet && page != nil {
		if contentType, body, ok := page(req.URL.Path); ok {
			response := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: %s\r\nContent-Length: %d\r\nCache-Control: no-store\r\nConnection: close\r\n\r\n", contentType, len(body))
			if err := websocketWriteAll(conn, append([]byte(response), body...)); err != nil {
				return nil, localIOError("WebSocket status response", err)
			}
			return nil, io.EOF
		}
	}
	key := req.Header.Get("Sec-WebSocket-Key")
	decoded, keyErr := base64.StdEncoding.Strict().DecodeString(key)
	// Subprotocols are case-sensitive, unlike HTTP upgrade tokens (RFC 6455 §4).
	protocol := false
	for _, value := range req.Header.Values("Sec-WebSocket-Protocol") {
		for _, part := range strings.Split(value, ",") {
			protocol = protocol || strings.TrimSpace(part) == websocketProtocol
		}
	}
	if !validGet || req.RequestURI != "/relay" || !websocketToken(req.Header, "Connection", "upgrade") || !websocketToken(req.Header, "Upgrade", "websocket") || len(req.Header.Values("Sec-WebSocket-Version")) != 1 || req.Header.Get("Sec-WebSocket-Version") != "13" || len(req.Header.Values("Sec-WebSocket-Key")) != 1 || keyErr != nil || len(decoded) != 16 || !protocol || len(req.Header.Values("Sec-WebSocket-Extensions")) != 0 {
		_, _ = io.WriteString(conn, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return nil, hostedError("WebSocket handshake", "a version-13 binary relay upgrade without extensions")
	}
	response := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + websocketAccept(key) + "\r\nSec-WebSocket-Protocol: " + websocketProtocol + "\r\n\r\n"
	if _, err := io.WriteString(conn, response); err != nil {
		return nil, localIOError("WebSocket upgrade", err)
	}
	return newWebSocketConn(conn, r, false, hostedMaxClientFrame+5, timeout, websocketPingInterval), nil
}

// DialHostedWebSocket verifies a wss server certificate and exchanges the
// hosted hello. ws is restricted to explicit numeric-loopback tests. Like
// DialHosted it returns the grant stream without a lobby (§16.6.1).
func DialHostedWebSocket(ctx context.Context, address, room string, hello LocalHello, options HostedDialOptions) (*LocalClient, string, error) {
	if !strings.Contains(address, "://") {
		return nil, "", hostedError("WebSocket URL", "ws(s)://host/relay without credentials, query or fragment")
	}
	return dialHostedStream(ctx, address, room, hello, options)
}

// dialWebSocketTransport validates a ws(s)://host/relay URL, dials it and
// completes the upgrade within the context's deadline.
func dialWebSocketTransport(ctx context.Context, address string, options HostedDialOptions) (net.Conn, error) {
	u, err := url.Parse(address)
	if err != nil || u == nil || (u.Scheme != "wss" && u.Scheme != "ws") || u.Opaque != "" || u.Hostname() == "" || u.User != nil || u.EscapedPath() != "/relay" || u.RawQuery != "" || u.ForceQuery || strings.Contains(address, "#") {
		return nil, hostedError("WebSocket URL", "ws(s)://host/relay without credentials, query or fragment")
	}
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "ws" {
			port = "80"
		}
	}
	target := net.JoinHostPort(u.Hostname(), port)
	if u.Scheme == "ws" {
		if !options.InsecureLoopback || options.TLSConfig != nil {
			return nil, hostedError("WebSocket TLS", "explicit numeric-loopback plaintext without TLS configuration")
		}
		if err := localAddress(target, false); err != nil {
			return nil, err
		}
	} else if options.InsecureLoopback || (options.TLSConfig != nil && options.TLSConfig.InsecureSkipVerify) {
		return nil, hostedError("WebSocket TLS", "verified TLS without the plaintext switch")
	}
	var raw net.Conn
	if u.Scheme == "ws" {
		raw, err = (&net.Dialer{}).DialContext(ctx, "tcp", target)
	} else {
		config := &tls.Config{}
		if options.TLSConfig != nil {
			config = options.TLSConfig.Clone()
		}
		config.MinVersion = max(config.MinVersion, tls.VersionTLS12)
		config.NextProtos = []string{"http/1.1"}
		raw, err = (&tls.Dialer{Config: config}).DialContext(ctx, "tcp", target)
	}
	if err != nil {
		return nil, localIOError("WebSocket dial", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	_ = raw.SetDeadline(deadline)
	_ = raw.SetWriteDeadline(minDeadline(deadline, time.Now().Add(hostedDefaultTimeouts.write)))
	stream, err := dialWebSocketUpgrade(raw, u.Host)
	if err != nil {
		_ = raw.Close()
		if ctx.Err() != nil {
			return nil, localIOError("WebSocket handshake", ctx.Err())
		}
		return nil, err
	}
	_ = stream.SetDeadline(deadline)
	return stream, nil
}

func dialWebSocketUpgrade(conn net.Conn, host string) (*websocketConn, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, localIOError("WebSocket nonce", err)
	}
	key := base64.StdEncoding.EncodeToString(nonce[:])
	request := fmt.Sprintf("GET /relay HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: %s\r\n\r\n", host, key, websocketProtocol)
	if _, err := io.WriteString(conn, request); err != nil {
		return nil, localIOError("WebSocket upgrade", err)
	}
	r := bufio.NewReader(conn)
	header, err := readWebSocketHeader(r)
	if err != nil {
		return nil, err
	}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(header)), nil)
	if err != nil {
		return nil, hostedError("WebSocket HTTP response", "a valid upgrade response")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols || response.ProtoMajor != 1 || response.ProtoMinor != 1 || !websocketToken(response.Header, "Connection", "upgrade") || !websocketToken(response.Header, "Upgrade", "websocket") || len(response.Header.Values("Sec-WebSocket-Accept")) != 1 || response.Header.Get("Sec-WebSocket-Accept") != websocketAccept(key) || len(response.Header.Values("Sec-WebSocket-Protocol")) != 1 || response.Header.Get("Sec-WebSocket-Protocol") != websocketProtocol || len(response.Header.Values("Sec-WebSocket-Extensions")) != 0 || response.ContentLength > 0 || len(response.TransferEncoding) != 0 {
		return nil, hostedError("WebSocket handshake", "a matching relay upgrade without extensions")
	}
	return newWebSocketConn(conn, r, true, localMaxGrantFrame+5, hostedDefaultTimeouts.write, 0), nil
}
