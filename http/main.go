package main

import (
	"errors"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/samiam2013/wiki4dummies/constants"
	"github.com/samiam2013/wiki4dummies/normalize"
	"github.com/samiam2013/wiki4dummies/wiki"
)

func main() {
	var savePath string
	flag.StringVar(&savePath, "save_path", "", "Path to the save index, page files")
	flag.Parse()

	if savePath == "" {
		fmt.Println("The save_path arg is required")
		return
	}

	fmt.Println("Initializing w4d server")
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/index.html")
	})
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./"+r.URL.Path)
	})
	mux.HandleFunc("/search", handleSearch(savePath))
	err := http.ListenAndServe(":3030", mux)
	fmt.Printf("Server stopped, error: %v\n", err)
}

type SearchPageData struct {
	Query   string
	Results []SearchResult
}

type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

func handleSearch(savePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if strings.TrimSpace(q) == "" {
			http.Error(w, "No query provided", http.StatusBadRequest)
			return
		}

		// data := mockSearch(q)
		// _ = savePath
		data, err := search(savePath, q)
		if err != nil {
			http.Error(w, "Failed to search", http.StatusInternalServerError)
			fmt.Printf("Failed to search: %v\n", err)
			return
		}

		// Parse and execute the template
		w.Header().Set("Content-Type", "text/html")
		tmpl := template.Must(template.ParseFiles("./results.tmpl"))
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

type idxRow struct {
	WordFreq   int
	ExactMatch bool
	RelPath    string
}

type index []idxRow

func loadIndex(idxPath string) (index, error) {
	f, err := os.Open(idxPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open index file: %w", err)
	}
	defer f.Close()

	// Read the index file
	rows := []idxRow{}
	for {
		var wordFreq int
		var exactMatch bool
		var relPath string
		_, err := fmt.Fscanf(f, "%d,%t,%s\n", &wordFreq, &exactMatch, &relPath)
		if err != nil {
			if err.Error() != "EOF" {
				fmt.Println("Error reading index file:", err)
			}
			break
		}
		rows = append(rows, idxRow{WordFreq: wordFreq, ExactMatch: exactMatch, RelPath: relPath})
	}
	return rows, nil
}

func search(savePath, q string) (SearchPageData, error) {
	// Search the index
	indexPath := filepath.Join(savePath, constants.IndexFileFolder)
	wordFreqs, err := wiki.GatherWordFrequency(strings.NewReader(q))
	if err != nil {
		return SearchPageData{}, fmt.Errorf("failed to gather word frequency: %w", err)
	}
	// stemmedQueryWords := normalize.StemmedWordFreqs(wordFreqs)

	spd := SearchPageData{Query: q, Results: []SearchResult{}}

	indexes := map[string]index{}
	// for each exact match word look for an index file
	for word := range wordFreqs {
		triePath, err := normalize.TrieMake(indexPath, word)
		if err != nil {
			return SearchPageData{}, fmt.Errorf("failed to make trie path: %w", err)
		}
		idxSavePath := filepath.Join(triePath, word+".idx")
		idxRows, err := loadIndex(idxSavePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// no index file, no results
				continue
			}
			return SearchPageData{}, fmt.Errorf("failed to load index: %w", err)
		}
		indexes[word] = idxRows
	}

	fmt.Printf("Indexes: %#v\n", indexes)

	// Load the page files
	// Search the page
	// rank the matching
	// Return the results
	return spd, nil
}

func mockSearch(q string) SearchPageData {
	results := []SearchResult{}
	for i := 1; i <= 25; i++ {
		results = append(results, SearchResult{
			Title: fmt.Sprintf("Result %d", i),
			URL:   fmt.Sprintf("http://example.com/%d", i),
			Snippet: fmt.Sprintf("This is a snippet of result %d. it's very long and it should wrap over if it's wider "+
				"than the page or something like that This is a snippet of result. it's very long and it should wrap"+
				" over if it's wider than the page or something like that", i),
		})
	}
	data := SearchPageData{
		Query:   q,
		Results: results,
	}
	return data
}
