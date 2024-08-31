package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/samiam2013/wiki4dummies/normalize"
	"github.com/samiam2013/wiki4dummies/wiki"
)

func main() {
	indexFolder := flag.String("index_folder", "../buildindex/index/", "Folder path to the index files")
	query := flag.String("query", "", "Search query")
	flag.Parse()
	slog.Info("search v0.01")
	slog.Info("Index folder set", "path", *indexFolder)

	if *query == "" {
		slog.Info("No search query provided, reading from stdin")
		in, err := io.ReadAll(os.Stdin)
		if err != nil {
			slog.Info("Error reading from stdin", "error", err)
			return
		}
		if len(in) == 0 {
			slog.Info("No search query provided")
			return
		}
		*query = string(in)
	}
	slog.Info("Search starting", "query", *query)

	// gather the terms from the query
	words := normalize.SplitAndLower(*query)
	// search the index for the words
	indexResults := make(map[string]map[string]int)
	for _, word := range words {
		if _, ok := wiki.FrequentWords[word]; ok {
			continue
		}
		if _, ok := indexResults[word]; ok {
			continue
		}
		fr, err := getIndexEntries(*indexFolder, word)
		if err != nil {
			slog.Error("Error getting index for word", "word", word, "error", err)
			continue
		}
		indexResults[word] = fr
	}
	// fmt.Printf("Results for search query: %v\n", indexResults)
	resultFileFreq := make(map[string]int)
	resultFileLowestFreq := make(map[string]int)
	for word, results := range indexResults {
		fmt.Printf("# results for word '%s': %d \n", word, len(results))
		for file, freq := range results {
			// fmt.Printf("%s: %d\n", file, freq)
			resultFileFreq[file] += 1
			_, ok := resultFileLowestFreq[file]
			if !ok || freq < resultFileLowestFreq[file] {
				resultFileLowestFreq[file] = freq
			}
		}
	}
	for file, freq := range resultFileFreq {
		lowestFreq := resultFileLowestFreq[file]
		if freq > 1 {
			fmt.Printf("%s found for %d words, lowest freq word %d\n", file, freq, lowestFreq)
		}
	}

	// get the frequency of the files (how many terms give back that filename)

	// sort the results by frequency and filename
}

func getIndexEntries(indexFolder, word string) (map[string]int, error) {
	filename, err := normalize.WordToStemmedFilename(word)
	if err != nil {
		return nil, fmt.Errorf("error getting filename for word: %w", err)
	}
	fh, err := os.OpenFile(indexFolder+filename, os.O_RDONLY, 0664)
	if err != nil {
		return nil, fmt.Errorf("error opening index file to get entries: %w", err)
	}
	defer func() { _ = fh.Close() }()
	s := bufio.NewScanner(fh)
	results := make(map[string]int)
	for s.Scan() {
		var file string
		var freq int
		if _, err := fmt.Sscanf(s.Text(), "%s %d", &file, &freq); err != nil {
			slog.Error("Error parsing index line", "error", err)
			continue
		}
		results[file] = freq
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("error scanning index file: %w", err)
	}
	return results, nil
}
