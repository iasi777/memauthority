// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package textcanon

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// CanonicalText matches the frozen Python canonical_text behavior used for
// managed Authority text: NFC, CRLF/CR to LF, and trailing Unicode whitespace
// removed from every logical line while preserving newline count.
func CanonicalText(value string) string {
	value = norm.NFC.String(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	return strings.Join(lines, "\n")
}

// FoldKey implements the frozen route comparison key: trim the query-like
// value, NFC normalize it, then apply Unicode case folding.
func FoldKey(value string) string {
	return cases.Fold().String(norm.NFC.String(strings.TrimSpace(value)))
}
