// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package publiccli

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPExposureAndTimeoutPolicy(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:8000", "localhost:8000", "[::1]:8000"} {
		if err := validateHTTPExposure(listen, false); err != nil {
			t.Fatalf("loopback %q refused: %v", listen, err)
		}
	}
	for _, listen := range []string{"0.0.0.0:8000", ":8000", "[::]:8000", "192.0.2.10:8000"} {
		if err := validateHTTPExposure(listen, false); err == nil {
			t.Fatalf("unauthenticated non-loopback %q accepted", listen)
		}
		if err := validateHTTPExposure(listen, true); err != nil {
			t.Fatalf("OAuth non-loopback %q refused: %v", listen, err)
		}
	}
	srv := newHTTPServer("127.0.0.1:0", http.NewServeMux())
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.WriteTimeout <= 0 || srv.IdleTimeout <= 0 || srv.MaxHeaderBytes <= 0 {
		t.Fatalf("defensive HTTP limits missing: %#v", srv)
	}
}

func TestCanonicalStateDirContainmentRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	vault := filepath.Join(base, "vault")
	if err := os.MkdirAll(vault, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "external-looking-state")
	if err := os.Symlink(vault, link); err != nil {
		t.Fatal(err)
	}
	inside, err := pathWithinCanonical(vault, link)
	if err != nil {
		t.Fatal(err)
	}
	if !inside {
		t.Fatal("symlinked state dir into Vault was not detected")
	}
}
