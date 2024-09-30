package normalize

import (
	"fmt"
	"os"
	"path/filepath"
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

// TrieMake creates a directory structure for the title with the first two characters
func TrieMake(savePath, title string) (string, error) {
	if len(title) < 3 {
		title = fmt.Sprintf("%3s", title)
		title = strings.ReplaceAll(title, " ", "_")
	}
	// create the path
	first := string(title[0]) // this might break on emoji
	second := string(title[1])
	path := filepath.Join(savePath, first, second)
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("failed to create parent directories: %w", err)
	}
	return path, nil
}
