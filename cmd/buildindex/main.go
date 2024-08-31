package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/caneroj1/stemmer"
	"github.com/flytam/filenamify"
	"github.com/samiam2013/wiki4dummies/wiki"
)

func main() {
	pagesFolder := flag.String("pages_folder", "../parsedump/pages/", "Folder to gather pages from")
	indexFolder := flag.String("index_folder", "./index/", "Folder to store the index")
	flag.Parse()

	pagesStat, err := os.Stat(*pagesFolder)
	if os.IsNotExist(err) {
		slog.Error("The pages folder does not exist")
		return
	} else if err != nil {
		slog.Error("Error checking the pages folder", "error", err)
		return
	}
	if !pagesStat.IsDir() {
		slog.Error("The pages folder is not a directory")
		return
	}
	pageFiles := make([]string, 0)
	if err := filepath.Walk(*pagesFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			slog.Error("Error walking the pages folder", "error", err)
			return err
		}
		if info.IsDir() {
			return nil
		}
		// skip the source files, we're building an index from the words,
		// the source files are so we can rebuild the data in html pages later
		if strings.HasSuffix(path, ".xml") {
			return nil
		}
		pageFiles = append(pageFiles, path)
		return nil
	}); err != nil {
		slog.Error("Error walking the pages folder", "error", err)
		return
	}
	if len(pageFiles) == 0 {
		slog.Error("No pages found in the pages folder")
		return
	}
	slog.Info("Found pages", "count", len(pageFiles))

	if _, err := os.Stat(*indexFolder); os.IsNotExist(err) {
		slog.Info("Creating the index folder")
		if err := os.Mkdir(*indexFolder, 0755); err != nil {
			slog.Error("Error creating the index folder", "error", err)
			return
		}
	} else if err != nil {
		slog.Error("Error checking the index folder", "error", err)
		return
	}

	for _, pageFile := range pageFiles {
		slog.Info("Processing page", "page", pageFile)
		// gather the word frequency of the page
		wordFreq, err := gatherWordFrequency(pageFile)
		if err != nil {
			slog.Error("Error gathering word frequency", "error", err)
			return
		}
		// remove the most frequent words
		for word := range wiki.FrequentWords {
			delete(wordFreq, strings.ToLower(word))
		}
		// get the stem of each word and write the frequency to that index file
		for word, freq := range wordFreq {
			stem := stemmer.Stem(word)

			stem, err = filenamify.Filenamify(stem, filenamify.Options{Replacement: "-"})
			if err != nil {
				slog.Error("Error filenamifying the stem for index file", "error", err)
				return
			}

			indexFile := *indexFolder + stem + ".txt"
			f, err := os.OpenFile(indexFile, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0664)
			if os.IsNotExist(err) {
				f, err = os.Create(indexFile)
				if err != nil {
					slog.Error("Error creating the index file", "error", err)
					return
				}
			} else if err != nil {
				slog.Error("Error opening the index file", "error", err)
				return
			}
			if _, err = fmt.Fprintf(f, "%s %d\n", word, freq); err != nil {
				slog.Error("Error writing to the index file", "error", err)
				return
			}
		}
		slog.Info("Processed page", "page", pageFile)
	}

}

func gatherWordFrequency(pageFile string) (map[string]int, error) {
	f, err := os.Open(pageFile)
	if err != nil {
		return nil, fmt.Errorf("Error opening the page file: %w", err)
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 64*1024), 100*1024)
	wordFreq := make(map[string]int)
	for s.Scan() {
		words := strings.Fields(s.Text())
		for _, word := range words {
			// remove the punctuation
			word = strings.Trim(word, ",.!?;:\"()[]{}")
			wordFreq[word]++
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("Error scanning the page file: %w", err)
	}
	return wordFreq, nil
}
