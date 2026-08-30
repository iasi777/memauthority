// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package publiccli

import (
	"bytes"
	"testing"
)

func TestVersionDiscovery(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		var out, errOut bytes.Buffer
		if code := Run(args, &out, &errOut); code != 0 {
			t.Fatalf("Run(%v) exit=%d stderr=%q", args, code, errOut.String())
		}
		if got := out.String(); got != "v-memory 1.3.0-dev\n" {
			t.Fatalf("Run(%v) stdout=%q", args, got)
		}
		if errOut.Len() != 0 {
			t.Fatalf("Run(%v) stderr=%q", args, errOut.String())
		}
	}
}

func TestVersionDiscoveryRejectsExtraArgs(t *testing.T) {
	for _, args := range [][]string{{"version", "extra"}, {"--version", "extra"}} {
		var out, errOut bytes.Buffer
		if code := Run(args, &out, &errOut); code != 2 {
			t.Fatalf("Run(%v) exit=%d", args, code)
		}
		if out.Len() != 0 {
			t.Fatalf("Run(%v) stdout=%q", args, out.String())
		}
	}
}

func TestMemAuthorityVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}} {
		var out, errOut bytes.Buffer
		if code := RunAs("memauthority", args, &out, &errOut); code != 0 {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, errOut.String())
		}
		if got := out.String(); got != "memauthority 1.3.0-dev\n" {
			t.Fatalf("args=%v output=%q", args, got)
		}
	}
}
