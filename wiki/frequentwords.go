package wiki

import (
	"bufio"
	"fmt"
	"io"

	"github.com/samiam2013/wiki4dummies/normalize"
)

var FrequentWords = map[string]struct{}{
	"the": {}, "be": {}, "to": {}, "and": {}, "a": {}, "an": {}, "of": {}, "i": {}, "in": {}, "that": {}, "you": {}, "have": {},
	"it": {}, "is": {}, "do": {}, "for": {}, "on": {}, "with": {}, "he": {}, "this": {}, "as": {}, "we": {}, "but": {}, "not": {},
	"they": {}, "what": {}, "at": {}, "my": {}, "his": {}, "get": {}, "go": {}, "from": {}, "will": {}, "say": {}, "can": {},
	"by": {}, "or": {}, "all": {}, "me": {}, "she": {}, "so": {}, "there": {}, "about": {}, "your": {}, "one": {}, "if": {}, "her": {}, "out": {},
	"just": {}, "when": {}, "like": {}, "up": {}, "who": {}, "make": {}, "would": {}, "no": {}, "their": {}, "time": {}, "see": {}, "more": {}, "know": {},
	"come": {}, "think": {}, "take": {}, "him": {}, "how": {}, "them": {}, "want": {}, "other": {}, "could": {},
	"now": {}, "year": {}, "look": {}, "right": {}, "into": {}, "people": {}, "our": {}, "which": {}, "then": {},
	"here": {}, "back": {}, "work": {}, "than": {}, "some": {}, "way": {}, "only": {}, "tell": {}, "because": {}, "good": {},
	"over": {}, "thing": {}, "use": {}, "need": {}, "two": {}, "day": {}, "even": {}, "these": {}, "where": {}, "give": {},
	"man": {}, "find": {}, "after": {}, "well": {}, "us": {}, "also": {}, "much": {}, "new": {}, "life": {}, "any": {},
	"first": {}, "should": {}, "call": {}, "down": {}, "most": {}, "those": {}, "very": {}, "too": {}, "why": {}, "feel": {},
	"really": {}, "through": {}, "try": {}, "never": {}, "before": {}, "something": {}, "many": {}, "let": {}, "help": {}, "little": {}, "off": {},
	"long": {}, "may": {}, "child": {}, "mean": {}, "woman": {}, "still": {}, "love": {}, "ask": {}, "great": {}, "show": {},
	"leave": {}, "around": {}, "world": {}, "talk": {}, "start": {}, "last": {}, "school": {}, "keep": {}, "own": {}, "put": {},
	"home": {}, "while": {}, "place": {}, "oh": {}, "another": {}, "big": {}, "turn": {}, "same": {}, "such": {}, "three": {}, "family": {},
	"again": {}, "change": {}, "play": {}, "both": {}, "each": {}, "always": {}, "high": {}, "old": {}, "every": {}, "point": {}, "hear": {},
	"run": {}, "state": {}, "away": {}, "happen": {}, "might": {}, "better": {}, "house": {}, "move": {}, "become": {}, "seem": {}, "hand": {},
	"between": {}, "end": {}, "yeah": {}, "friend": {}, "live": {}, "name": {}, "few": {}, "sure": {}, "believe": {}, "night": {},
	"since": {}, "problem": {}, "best": {}, "part": {}, "yes": {}, "guy": {}, "bad": {}, "far": {}, "hold": {}, "stop": {}, "next": {},
	"bring": {}, "week": {}, "ever": {}, "head": {}, "without": {}, "lot ": {}}

func GatherWordFrequency(r io.Reader) (map[string]int, error) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 100*100*1024)
	wordFreq := make(map[string]int)
	for s.Scan() {
		words := normalize.SplitAndLower(s.Text())
		for _, word := range words {
			if _, ok := FrequentWords[word]; ok {
				continue
			}
			wordFreq[word]++
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("failed scanning the page file: %w", err)
	}
	return wordFreq, nil
}
