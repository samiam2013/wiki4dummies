package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"

	"github.com/adrg/strutil/metrics"
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

	results := make(fileResults, 0)
	for file, freq := range resultFileFreq {
		lowestFreq := resultFileLowestFreq[file]
		// if freq > 0 { // TODO should this be > 0 ?
		// TODO add the levenshtein distance from the query string to the file name on each result
		lev := metrics.NewLevenshtein()
		dist := lev.Distance(*query, file)
		results = append(results,
			fileResult{name: file, termsMatched: freq, lowestFreq: lowestFreq, queryNameDist: dist})
		// fmt.Printf("%s found for %d words, lowest freq word %d\n", file, freq, lowestFreq)
		// }
	}
	results.Sort()
	// fmt.Printf("Results: %#v\n", results)

	// get the frequency of the files (how many terms give back that filename)
	const resultLimit = 10
	for i, result := range results {
		if i >= resultLimit {
			break
		}
		fmt.Printf("%s found for %d words, lowest freq word %d\n", result.name, result.termsMatched, result.lowestFreq)
	}

	// sort the results by frequency and filename
}

type fileResult struct {
	name          string
	queryNameDist int
	termsMatched  int
	lowestFreq    int
}
type fileResults []fileResult

// sort the results by most terms matched,  lowest text distance,  and largest minimum frequency in that order of importance
func (fr fileResults) Sort() {
	// sort.Slice(fr, func(i, j int) bool {
	// 	if fr[i].termsMatched == fr[j].termsMatched {
	// 		return fr[i].lowestFreq > fr[j].lowestFreq
	// 	}
	// 	return fr[i].termsMatched < fr[j].termsMatched
	// })
	sort.Slice(fr, func(i, j int) bool {
		if fr[i].termsMatched == fr[j].termsMatched {
			if fr[i].queryNameDist == fr[j].queryNameDist {
				return fr[i].lowestFreq > fr[j].lowestFreq
			}
			return fr[i].queryNameDist < fr[j].queryNameDist
		}
		return fr[i].termsMatched > fr[j].termsMatched
	})
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
