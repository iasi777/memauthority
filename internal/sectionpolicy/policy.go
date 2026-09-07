// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

// Package sectionpolicy owns the ordinary section mutation capabilities.
package sectionpolicy

import "strings"

const VerificationHeading = "核验记录"

const EditDescription = "handoff and rules allow replace, append, insert, delete; progress and pitfalls allow replace, delete (create entries with memory_append_progress or memory_record_pitfall). insert requires a new exact H2 heading; replace/append/delete require an existing heading and never create one. 核验记录 is protected: use memory_mark_verified. handoff also supports checklist_remove and checklist_set_checked under 已知问题 / 待办; do not mix checklist and H2 operations in one batch."

func Operations(role string) []string {
	switch role {
	case "handoff":
		return []string{"replace", "append", "insert", "delete", "checklist_remove", "checklist_set_checked"}
	case "rules":
		return []string{"replace", "append", "insert", "delete"}
	case "progress", "pitfalls":
		return []string{"replace", "delete"}
	}
	return []string{}
}

func Allowed(role, operation string) bool {
	for _, candidate := range Operations(role) {
		if candidate == operation {
			return true
		}
	}
	return false
}

// Protected preserves the existing qualified-heading verification guard.
func Protected(role, heading string) bool {
	if role != "handoff" {
		return false
	}
	if i := strings.LastIndex(heading, " / "); i >= 0 {
		heading = heading[i+3:]
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(heading), "## ")) == VerificationHeading
}

type Section struct {
	Heading           string   `json:"heading"`
	AllowedOperations []string `json:"allowed_operations"`
	Protected         bool     `json:"protected"`
	DedicatedTool     string   `json:"dedicated_tool,omitempty"`
}

// Describe reports resource-wide structure, even when a read selects a page.
// The headings use the same H2 recognition as ordinary section mutations.
func Describe(text, role string) []Section {
	result := make([]Section, 0)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimPrefix(line, "## "))
		section := Section{Heading: heading, AllowedOperations: []string{}}
		if Protected(role, heading) {
			section.Protected = true
			section.DedicatedTool = "memory_mark_verified"
		} else {
			for _, operation := range Operations(role) {
				if operation == "insert" {
					continue
				}
				if strings.HasPrefix(operation, "checklist_") && heading != "已知问题 / 待办" {
					continue
				}
				section.AllowedOperations = append(section.AllowedOperations, operation)
			}
		}
		result = append(result, section)
	}
	return result
}

func CreationHint(role string) string {
	if Allowed(role, "insert") {
		return "To create a missing ordinary section, call memory_update_sections with operation=insert, the exact new H2 heading, content, and this resource revision. Do not retry replace or append on a missing section."
	}
	if role == "progress" {
		return "Create progress entries with memory_append_progress."
	}
	if role == "pitfalls" {
		return "Create pitfall entries with memory_record_pitfall."
	}
	return ""
}
