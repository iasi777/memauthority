// Copyright 2026 iasi777
// SPDX-License-Identifier: Apache-2.0

package vaultread

import (
	"math"
	"math/bits"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	lexicalResultLimit   = 10
	lexicalFetchLimit    = lexicalResultLimit + 1
	maxLexicalFeatures   = 128
	maxLexicalQueryRunes = 4096
)

var searchCaseFolder = cases.Fold()

type lexicalSpan struct {
	start int
	end   int
}

type lexicalFeature struct {
	text string
	span lexicalSpan
}

type lexicalPlan struct {
	featureIndex map[string]int
	spans        []lexicalSpan
}

type lexicalCandidate struct {
	rowID        int64
	matches      [2]uint64
	projectOrder int
	roleOrder    int
	lineStart    int
}

type rankedSearchHit struct {
	lexicalCandidate
	score float64
}

func (p *projection) searchLexicalOverlap(query, projectID string, crossProject bool) ([]map[string]any, error) {
	plan := buildLexicalPlan(query)
	// lexical_overlap still identifies the attempted fallback mode when a query
	// has no eligible lexical features; in that case the fallback returns no hits.
	if len(plan.spans) == 0 {
		return []map[string]any{}, nil
	}

	statement := `SELECT id, project_id, role, heading, line_start, line_end, content, section_hash, resource_revision, resource_uri, source_revision FROM sections WHERE heading<>''`
	var args []any
	if !crossProject {
		statement += ` AND project_id=?`
		args = append(args, projectID)
	}
	statement += ` ORDER BY project_id, role, line_start`
	rows, err := p.db.Query(statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	documentFrequency := make([]int, len(plan.spans))
	documentCount := 0
	var candidates []lexicalCandidate
	lastProject := ""
	projectOrder := -1
	for rows.Next() {
		var rowID int64
		var section indexedSection
		if err := rows.Scan(
			&rowID, &section.ProjectID, &section.Role, &section.Heading, &section.LineStart, &section.LineEnd,
			&section.Content, &section.SectionHash, &section.ResourceRevision, &section.ResourceURI, &section.SourceRevision,
		); err != nil {
			return nil, err
		}
		documentCount++
		if section.ProjectID != lastProject {
			projectOrder++
			lastProject = section.ProjectID
		}
		matches := plan.match(section.Content)
		if matches[0] == 0 && matches[1] == 0 {
			continue
		}
		forEachLexicalMatch(matches, func(index int) { documentFrequency[index]++ })
		candidates = append(candidates, lexicalCandidate{
			rowID: rowID, matches: matches, projectOrder: projectOrder,
			roleOrder: searchRoleRank(section.Role), lineStart: section.LineStart,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if documentCount == 0 || len(candidates) == 0 {
		return []map[string]any{}, nil
	}

	weights := make([]float64, len(plan.spans))
	for i, frequency := range documentFrequency {
		if frequency > 0 {
			weights[i] = 1 + math.Log(float64(documentCount)/float64(frequency))
		}
	}
	top := make([]rankedSearchHit, 0, lexicalFetchLimit)
	for _, candidate := range candidates {
		score := plan.score(candidate.matches, weights)
		if score > 0 {
			top = insertRankedHit(top, rankedSearchHit{lexicalCandidate: candidate, score: score})
		}
	}
	return p.fetchLexicalResults(top)
}

func (p *projection) fetchLexicalResults(top []rankedSearchHit) ([]map[string]any, error) {
	if len(top) == 0 {
		return []map[string]any{}, nil
	}
	placeholders := make([]string, len(top))
	args := make([]any, len(top))
	for i, hit := range top {
		placeholders[i] = "?"
		args[i] = hit.rowID
	}
	statement := `SELECT id, project_id, role, heading, line_start, line_end, content, section_hash, resource_revision, resource_uri, source_revision FROM sections WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := p.db.Query(statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := make(map[int64]map[string]any, len(top))
	for rows.Next() {
		var rowID int64
		var section indexedSection
		if err := rows.Scan(
			&rowID, &section.ProjectID, &section.Role, &section.Heading, &section.LineStart, &section.LineEnd,
			&section.Content, &section.SectionHash, &section.ResourceRevision, &section.ResourceURI, &section.SourceRevision,
		); err != nil {
			return nil, err
		}
		byID[rowID] = searchResult(section, matchLexicalOverlap)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0, len(top))
	for _, hit := range top {
		if result := byID[hit.rowID]; result != nil {
			results = append(results, result)
		}
	}
	return results, nil
}

func insertRankedHit(top []rankedSearchHit, hit rankedSearchHit) []rankedSearchHit {
	position := 0
	for position < len(top) && rankedHitBefore(top[position], hit) {
		position++
	}
	if position >= lexicalFetchLimit {
		return top
	}
	top = append(top, rankedSearchHit{})
	copy(top[position+1:], top[position:])
	top[position] = hit
	if len(top) > lexicalFetchLimit {
		top = top[:lexicalFetchLimit]
	}
	return top
}

func rankedHitBefore(left, right rankedSearchHit) bool {
	if left.score != right.score {
		return left.score > right.score
	}
	if left.projectOrder != right.projectOrder {
		return left.projectOrder < right.projectOrder
	}
	if left.roleOrder != right.roleOrder {
		return left.roleOrder < right.roleOrder
	}
	return left.lineStart < right.lineStart
}

func searchRoleRank(role string) int {
	for i, candidate := range projectionRoleOrder {
		if role == candidate {
			return i
		}
	}
	return len(projectionRoleOrder)
}

func buildLexicalPlan(query string) lexicalPlan {
	query = foldSearchText(limitLexicalQuery(query))
	seen := map[string]bool{}
	all := make([]lexicalFeature, 0)
	offset := 0
	for _, segment := range lexicalSegments(query) {
		runes := []rune(segment)
		add := func(text string, start, end int) {
			if text == "" || seen[text] {
				return
			}
			seen[text] = true
			all = append(all, lexicalFeature{text: text, span: lexicalSpan{start: offset + start, end: offset + end}})
		}
		if minimum := asciiSearchTokenMinimum(segment); minimum > 0 {
			if len(runes) >= minimum {
				coverage := len(runes)
				if coverage > 3 {
					coverage = 3
				}
				add(segment, 0, coverage)
			}
		} else {
			for n := 2; n <= 3; n++ {
				for i := 0; i+n <= len(runes); i++ {
					add(string(runes[i:i+n]), i, i+n)
				}
			}
		}
		offset += len(runes)
	}
	if len(all) > maxLexicalFeatures {
		sampled := make([]lexicalFeature, 0, maxLexicalFeatures)
		for i := 0; i < maxLexicalFeatures; i++ {
			sampled = append(sampled, all[i*(len(all)-1)/(maxLexicalFeatures-1)])
		}
		all = sampled
	}
	plan := lexicalPlan{
		featureIndex: make(map[string]int, len(all)),
		spans:        make([]lexicalSpan, len(all)),
	}
	for i, feature := range all {
		plan.featureIndex[feature.text] = i
		plan.spans[i] = feature.span
	}
	return plan
}

func (plan lexicalPlan) match(content string) [2]uint64 {
	var matches [2]uint64
	mark := func(feature string) {
		index, ok := plan.featureIndex[feature]
		if !ok {
			return
		}
		matches[index/64] |= uint64(1) << uint(index%64)
	}
	for _, segment := range lexicalSegments(foldSearchText(content)) {
		runes := []rune(segment)
		if minimum := asciiSearchTokenMinimum(segment); minimum > 0 {
			if len(runes) >= minimum {
				mark(segment)
			}
			continue
		}
		for n := 2; n <= 3; n++ {
			for i := 0; i+n <= len(runes); i++ {
				mark(string(runes[i : i+n]))
			}
		}
	}
	return matches
}

func (plan lexicalPlan) score(matches [2]uint64, weights []float64) float64 {
	type weightedSpan struct {
		lexicalSpan
		weight float64
	}
	var matchedStorage [maxLexicalFeatures]weightedSpan
	matched := matchedStorage[:0]
	var pointStorage [maxLexicalFeatures * 2]int
	points := pointStorage[:0]
	forEachLexicalMatch(matches, func(index int) {
		if weights[index] == 0 {
			return
		}
		span := plan.spans[index]
		matched = append(matched, weightedSpan{lexicalSpan: span, weight: weights[index]})
		points = append(points, span.start, span.end)
	})
	if len(points) == 0 {
		return 0
	}
	sort.Ints(points)
	unique := points[:1]
	for _, point := range points[1:] {
		if point != unique[len(unique)-1] {
			unique = append(unique, point)
		}
	}
	score := 0.0
	for i := 0; i+1 < len(unique); i++ {
		start, end := unique[i], unique[i+1]
		weight := 0.0
		for _, span := range matched {
			if span.start <= start && span.end >= end && span.weight > weight {
				weight = span.weight
			}
		}
		score += weight * float64(end-start)
	}
	return score
}

func limitLexicalQuery(input string) string {
	runes := 0
	for index := range input {
		if runes == maxLexicalQueryRunes {
			return input[:index]
		}
		runes++
	}
	return input
}

func forEachLexicalMatch(matches [2]uint64, visit func(int)) {
	for word, value := range matches {
		for value != 0 {
			bit := bits.TrailingZeros64(value)
			visit(word*64 + bit)
			value &= value - 1
		}
	}
}

func foldSearchText(input string) string {
	return searchCaseFolder.String(norm.NFC.String(input))
}

type lexicalSegmentKind uint8

const (
	lexicalSegmentNone lexicalSegmentKind = iota
	lexicalSegmentASCII
	lexicalSegmentUnicode
)

func lexicalSegments(input string) []string {
	var segments []string
	var current []rune
	kind := lexicalSegmentNone
	flush := func() {
		if len(current) > 0 {
			segment := strings.Trim(string(current), "_-./")
			if segment != "" {
				segments = append(segments, segment)
			}
		}
		current = nil
		kind = lexicalSegmentNone
	}
	for _, r := range input {
		currentKind := lexicalSegmentNone
		switch {
		case r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_-./", r)):
			currentKind = lexicalSegmentASCII
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			currentKind = lexicalSegmentUnicode
		}
		if currentKind == lexicalSegmentNone {
			flush()
			continue
		}
		if kind != lexicalSegmentNone && kind != currentKind {
			flush()
		}
		kind = currentKind
		current = append(current, r)
	}
	flush()
	return segments
}

func asciiSearchTokenMinimum(input string) int {
	digitsOnly := input != ""
	for _, r := range input {
		if r > unicode.MaxASCII {
			return 0
		}
		if r < '0' || r > '9' {
			digitsOnly = false
		}
	}
	if digitsOnly {
		return 2
	}
	return 3
}
