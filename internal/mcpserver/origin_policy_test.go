// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostOriginPolicyThroughLoopbackProxy(t *testing.T) {
	root := fixtureRepo(t)
	work := t.TempDir()
	app, err := OpenWithOptions(root, work, OpenOptions{
		AllowedHosts:   []string{"127.0.0.1:8765", "localhost:8765", "v-memory.example.com"},
		AllowedOrigins: []string{"http://127.0.0.1:8765", "https://v-memory.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	request := func(host, origin string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
		r.Host = host
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		ctx := context.WithValue(r.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8765})
		return r.WithContext(ctx)
	}

	allowed := httptest.NewRecorder()
	app.HTTPHandler().ServeHTTP(allowed, request("v-memory.example.com", "https://v-memory.example.com"))
	if allowed.Code == http.StatusForbidden {
		t.Fatalf("explicitly allowed public Host/Origin blocked through loopback proxy: %d %s", allowed.Code, allowed.Body.String())
	}
	badHost := httptest.NewRecorder()
	app.HTTPHandler().ServeHTTP(badHost, request("evil.example", "https://v-memory.example.com"))
	if badHost.Code != http.StatusForbidden {
		t.Fatalf("unlisted Host accepted: %d %s", badHost.Code, badHost.Body.String())
	}
	badOrigin := httptest.NewRecorder()
	app.HTTPHandler().ServeHTTP(badOrigin, request("v-memory.example.com", "https://evil.example"))
	if badOrigin.Code != http.StatusForbidden {
		t.Fatalf("unlisted Origin accepted: %d %s", badOrigin.Code, badOrigin.Body.String())
	}
}
