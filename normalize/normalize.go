package normalize

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/caneroj1/stemmer"
	"github.com/flytam/filenamify"
)

func WordToStemmedFilename(word string) (string, error) {
	stem := stemmer.Stem(word)
	stem, err := filenamify.Filenamify(stem, filenamify.Options{Replacement: "-"})
	if err != nil {
		return "", fmt.Errorf("failed filenamifying the stem for index file: %w", err)
	}
	// some languages don't use spaces, so the stem could be the whole sentence
	if len(stem) > 50 {
		stem = stem[:50]
	}
	stem += ".txt"
	return stem, nil
}

var _reGetLowerWords = regexp.MustCompile(`[a-zA-Z]+`)

func SplitAndLower(s string) []string {
	words := make([]string, 0)
	for _, match := range _reGetLowerWords.FindAllString(s, -1) {
		words = append(words, strings.ToLower(match))
	}
	return words
}

func RemoveMarkup(s string) string {
	// remove the {{ }} and [[ ]]
	// s = strings.ReplaceAll(s, "{{", "")
	// s = strings.ReplaceAll(s, "}}", "")
	// s = strings.ReplaceAll(s, "[[", "")
	// s = strings.ReplaceAll(s, "]]", "")
	return s
}
