// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

// Command serverjson renders Official MCP Registry metadata from MCPB artifacts.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type transport struct {
	Type string `json:"type"`
}
type pkg struct {
	RegistryType string    `json:"registryType"`
	Identifier   string    `json:"identifier"`
	Version      string    `json:"version"`
	FileSHA256   string    `json:"fileSha256"`
	Transport    transport `json:"transport"`
}
type repository struct {
	URL    string `json:"url"`
	Source string `json:"source"`
	ID     string `json:"id"`
}
type server struct {
	Schema      string     `json:"$schema"`
	Name        string     `json:"name"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Version     string     `json:"version"`
	Repository  repository `json:"repository"`
	Packages    []pkg      `json:"packages"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: serverjson <version> <artifact-dir>")
		os.Exit(2)
	}
	version, dir := os.Args[1], os.Args[2]
	suffixes := []string{"win-x64", "win-arm64", "linux-x64", "linux-arm64", "osx-x64", "osx-arm64"}
	packages := make([]pkg, 0, len(suffixes))
	for _, suffix := range suffixes {
		name := fmt.Sprintf("memauthority-%s-%s.mcpb", version, suffix)
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			panic(err)
		}
		sum := sha256.Sum256(raw)
		packages = append(packages, pkg{
			RegistryType: "mcpb",
			Identifier:   fmt.Sprintf("https://github.com/iasi777/memauthority/releases/download/v%s/%s", version, name),
			Version:      version,
			FileSHA256:   hex.EncodeToString(sum[:]),
			Transport:    transport{Type: "stdio"},
		})
	}
	out := server{
		Schema:      "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
		Name:        "io.github.iasi777/memauthority",
		Title:       "MemAuthority",
		Description: "Git-backed long-term memory authority for AI agents via MCP.",
		Version:     version,
		Repository:  repository{URL: "https://github.com/iasi777/memauthority", Source: "github", ID: "1349426409"},
		Packages:    packages,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		panic(err)
	}
}
