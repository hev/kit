package search

import "strings"

// NormalizePhrase folds Unicode case with strings.ToLower and collapses runs
// of Unicode whitespace to one ASCII space. Punctuation and accents stay intact.
func NormalizePhrase(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// MatchesPhrase tests a contiguous substring of normalized text, not a bag of
// tokens. It does not join separate rows or require word boundaries.
func MatchesPhrase(text, phrase string) bool {
	needle := NormalizePhrase(phrase)
	return needle != "" && strings.Contains(NormalizePhrase(text), needle)
}
