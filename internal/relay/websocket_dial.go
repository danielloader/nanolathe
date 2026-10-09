//go:build !js

package relay

import (
	"context"
	"crypto/tls"
	"net"
	"time"
)

// dialWebSocketTransport validates a ws(s)://host/relay URL, dials it and
// completes the upgrade within the context's deadline. The browser build
// uses the browser's own WebSocket instead (websocket_browser_js.go).
func dialWebSocketTransport(ctx context.Context, address string, options HostedDialOptions) (net.Conn, error) {
	u, target, err := hostedWebSocketURL(address, options)
	if err != nil {
		return nil, err
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

// dialHostedSocket opens a verified TLS stream to host:port, or plaintext
// with the explicit numeric-loopback switch, with the context's deadline
// applied.
func dialHostedSocket(ctx context.Context, address string, options HostedDialOptions) (net.Conn, error) {
	if options.InsecureLoopback {
		if options.TLSConfig != nil {
			return nil, hostedError("TLS", "TLS or explicit loopback plaintext, not both")
		}
		if err := localAddress(address, false); err != nil {
			return nil, err
		}
	} else if options.TLSConfig != nil && options.TLSConfig.InsecureSkipVerify {
		return nil, hostedError("TLS", "server certificate verification")
	}
	var conn net.Conn
	var err error
	if options.InsecureLoopback {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", address)
	} else {
		config := &tls.Config{}
		if options.TLSConfig != nil {
			config = options.TLSConfig.Clone()
		}
		config.MinVersion = max(config.MinVersion, tls.VersionTLS12)
		conn, err = (&tls.Dialer{Config: config}).DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return nil, localIOError("hosted dial", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	return conn, nil
}
