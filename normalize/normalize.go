package normalize

import (
	"regexp"
	"strings"

	"github.com/caneroj1/stemmer"
)

var _reGetLowerWords = regexp.MustCompile(`[a-zA-Z]+`)

func SplitAndLower(s string) []string {
	words := make([]string, 0)
	for _, match := range _reGetLowerWords.FindAllString(s, -1) {
		words = append(words, strings.ToLower(match))
	}
	return words
}

// StemmedWordFreqs returns a map of stemmed words to their frequencies.
// stemming generally means uppercasing, this returns the lowercased version.
func StemmedWordFreqs(wordFreqs map[string]int) map[string]int {
	stemmedWordFreqs := make(map[string]int)
	for word, freq := range wordFreqs {
		stem := stemmer.Stem(word)
		stem = strings.ToLower(stem)
		stemmedWordFreqs[stem] += freq
	}
	return stemmedWordFreqs
}
