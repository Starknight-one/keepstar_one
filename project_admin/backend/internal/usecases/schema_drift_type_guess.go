package usecases

import (
	"regexp"
	"strconv"
	"strings"

	"keepstar-admin/internal/domain"
)

// GuessFieldType is a deterministic type heuristic for one unmapped
// inbox field. Used by the drift classifier to give the LLM a hint
// without burning tokens on type inference.
//
// Returns one of:
//
//	"numeric"     — every sample parses as a number
//	"date"        — every sample matches a date-ish pattern
//	"categorical" — small distinct set relative to non_null_count
//	"text"        — long strings, low repetition
//	"unknown"     — empty / no signal
func GuessFieldType(stats *domain.InboxFieldStats) string {
	if stats == nil || stats.NonNullCount == 0 {
		return "unknown"
	}
	samples := stats.SampleValues
	if len(samples) == 0 {
		return "unknown"
	}

	// Numeric — every non-empty sample parses as float.
	allNumeric := true
	for _, s := range samples {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			allNumeric = false
			break
		}
	}
	if allNumeric {
		return "numeric"
	}

	// Date-ish: ISO 8601, common slash formats, or RFC3339-ish.
	allDate := true
	for _, s := range samples {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !looksLikeDate(s) {
			allDate = false
			break
		}
	}
	if allDate {
		return "date"
	}

	// Categorical — small distinct set relative to row count.
	// "Small" = <= 50 distinct AND distinct/non_null ratio under 0.3.
	if stats.DistinctCount > 0 && stats.DistinctCount <= 50 {
		ratio := float64(stats.DistinctCount) / float64(stats.NonNullCount)
		if ratio < 0.3 {
			return "categorical"
		}
	}

	return "text"
}

var dateRegex = regexp.MustCompile(`^(?i)(\d{4}[-/]\d{1,2}[-/]\d{1,2}|\d{1,2}[-/]\d{1,2}[-/]\d{2,4})(T?\s?\d{1,2}:\d{2}(:\d{2})?(\.\d+)?(Z|[+-]\d{2}:?\d{2})?)?$`)

func looksLikeDate(s string) bool {
	return dateRegex.MatchString(s)
}
