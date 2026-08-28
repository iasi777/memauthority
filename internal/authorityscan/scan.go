// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package authorityscan

import (
	"regexp"
	"strings"
)

var highConfidenceSecretRE = regexp.MustCompile(`(?i)(?:^|[\s"'` + "`" + `])(?:api[_-]?key|access[_-]?token|secret|password)\s*[:=]\s*[^\s]+`)

func ContainsHighConfidenceSecret(text string) bool {
	return highConfidenceSecretRE.FindStringIndex(text) != nil
}

// HighConfidenceSecretLines returns one-based line numbers only. It never
// returns matched text, so callers can surface safe diagnostics without
// persisting secret material.
func HighConfidenceSecretLines(raw []byte) []int {
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\r", "\n"), "\n")
	out := make([]int, 0)
	for i, line := range lines {
		if highConfidenceSecretRE.FindStringIndex(line) != nil {
			out = append(out, i+1)
		}
	}
	return out
}
