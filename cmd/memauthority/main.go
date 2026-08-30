// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	"github.com/iasi777/memauthority/internal/publiccli"
)

func main() {
	os.Exit(publiccli.RunAs("memauthority", os.Args[1:], os.Stdout, os.Stderr))
}
