package main

import (
	"bufio"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/samiam2013/wiki4dummies/constants"
	"github.com/samiam2013/wiki4dummies/normalize"
	"github.com/samiam2013/wiki4dummies/wiki"
	"github.com/semantosoph/gowiki"
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
	mux.HandleFunc("/page/", handlePage(savePath))
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
		if _, err := fmt.Fscanf(f, "%d,%t,%s\n", &wordFreq, &exactMatch, &relPath); err != nil {
			if err.Error() != "EOF" {
				fmt.Println("Error reading index file:", err)
				continue
			}
			break
		}
		rows = append(rows, idxRow{WordFreq: wordFreq, ExactMatch: exactMatch, RelPath: relPath})
	}
	return rows, nil
}

const ExactMatchMultiplier = 3

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
	pages := map[string]int{}
	// for each exact match word look for an index file
	for word := range wordFreqs {
		triePath, err := normalize.TrieMake(indexPath, word)
		if err != nil {
			return SearchPageData{}, fmt.Errorf("failed to make trie path: %w", err)
		}
		idxSavePath := filepath.Join(triePath, word+".idx")
		idxRows, err := loadIndex(idxSavePath)
		if errors.Is(err, os.ErrNotExist) {
			// no index file, no results
			continue
		}
		if err != nil {
			return SearchPageData{}, fmt.Errorf("failed to load index: %w", err)
		}
		indexes[word] = idxRows
		for _, row := range idxRows {
			if row.ExactMatch {
				pages[row.RelPath] += ExactMatchMultiplier * row.WordFreq
				continue
			}
			pages[row.RelPath] += row.WordFreq
		}
	}

	// sort the pages into a list of matches by index score
	pagesByNumMatches := map[int][]string{}
	maxScore := 0
	for relPath, score := range pages {
		pagesByNumMatches[score] = append(pagesByNumMatches[score], relPath)
		if score > maxScore {
			maxScore = score
		}
	}

	const maxResults = 100
	topResults := []string{}
	for score := maxScore; score >= 0; score-- {
		if len(pagesByNumMatches[score]) == 0 {
			continue
		}
		for _, relPath := range pagesByNumMatches[score] {
			topResults = append(topResults, relPath)
			if len(topResults) >= maxResults {
				break
			}
		}
		if len(topResults) >= maxResults {
			break
		}
	}

	type match struct {
		relPath    string
		indexScore int
		textScore  int
	}
	// for each page, load the page file and search for the query
	matchList := []match{}
	for relPath, score := range pages {
		// TODO this is kludgy, should be removed earlier or something
		if !slices.Contains(topResults, relPath) {
			continue
		}
		var m match
		m.relPath = relPath
		m.indexScore = score
		textScore, err := scorePageMatch(filepath.Join(savePath, constants.PageFileFolder, relPath), q)
		if err != nil {
			fmt.Println("Failed to score page match:", err)
			continue
		}
		m.textScore = textScore
		matchList = append(matchList, m)
	}

	sort.Slice(matchList, func(i, j int) bool {
		if matchList[i].indexScore+matchList[i].textScore == matchList[j].indexScore+matchList[j].textScore {
			return matchList[i].indexScore > matchList[j].indexScore
		}
		return matchList[i].indexScore+matchList[i].textScore > matchList[j].indexScore+matchList[j].textScore
	})

	spd = SearchPageData{Query: q, Results: []SearchResult{}}
	for _, m := range matchList {
		// fmt.Printf("Match: %s, indexScore: %d, textScore: %d\n", m.relPath, m.indexScore, m.textScore)
		var sr SearchResult
		filePath := filepath.Join(savePath, constants.PageFileFolder, m.relPath)
		fh, err := os.Open(filePath)
		if err != nil {
			fmt.Println("Failed to open page file:", err)
			continue
		}
		pageBuffer, err := io.ReadAll(fh)
		if err != nil {
			fmt.Println("Failed to read page file:", err)
			continue
		}
		title, abstract, text, err := parsePage(pageBuffer)
		if err != nil {
			fmt.Println("Failed to parse page:", err)
			continue
		}
		sr.Title = title
		sr.URL = fmt.Sprintf("/page/%s", m.relPath)
		if abstract != "" {
			sr.Snippet = abstract
		} else {
			sr.Snippet = text
		}
		const snippetMaxLen = 300
		if len(sr.Snippet) > snippetMaxLen {
			sr.Snippet = sr.Snippet[:snippetMaxLen]
			lastSpace := strings.LastIndex(sr.Snippet, " ")
			sr.Snippet = sr.Snippet[:lastSpace]
			sr.Snippet += "..."
		}
		spd.Results = append(spd.Results, sr)

	}

	// Load the page files
	// Search the page
	// rank the matching
	// Return the results
	return spd, nil
}

// parsePage returns title, abstract, text, error and only contains the text if
// it was not able to parse the abstract
func parsePage(pageBuffer []byte) (string, string, string, error) {
	var page wiki.Page
	if err := xml.Unmarshal(pageBuffer, &page); err != nil {
		return "", "", "", fmt.Errorf("failed to unmarshal page: %w", err)
	}

	article, err := gowiki.ParseArticle(page.Title, page.Revision.Text.Text, &gowiki.DummyPageGetter{})
	if err != nil {
		return "", "", "", fmt.Errorf("failed to parse article: %w", err)
	}

	abstract := article.GetAbstract()
	abstract = strings.ReplaceAll(abstract, "\n", "")
	pageText := article.GetText()

	return page.Title, abstract, pageText, nil
}

func scorePageMatch(pagePath, q string) (int, error) {
	words := strings.Fields(strings.ToLower(q))
	f, err := os.Open(pagePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open page file: %w", err)
	}
	defer f.Close()
	var matches int
	// scan over each line and count the number of times the words appear
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.ToLower(sc.Text())
		for _, word := range words {
			matches += strings.Count(line, word)
		}
	}
	if err := sc.Err(); err != nil {
		return 0, fmt.Errorf("failed to scan page file: %w", err)
	}
	return matches, nil
}

func handlePage(savePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relPath := strings.TrimPrefix(r.URL.Path, "/page/")
		if relPath == "" {
			http.Error(w, "No page provided", http.StatusBadRequest)
			return
		}

		// Load the page file
		pagePath := filepath.Join(savePath, constants.PageFileFolder, relPath)
		f, err := os.Open(pagePath)
		if err != nil {
			http.Error(w, "Failed to open page", http.StatusInternalServerError)
			fmt.Printf("Failed to open page: %v\n", err)
			return
		}
		defer f.Close()

		pageBuffer, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "Failed to read page", http.StatusInternalServerError)
			fmt.Printf("Failed to read page: %v\n", err)
			return
		}
		// Execute the template
		w.Header().Set("Content-Type", "text")
		if _, err := w.Write(pageBuffer); err != nil {
			http.Error(w, "Failed to write page", http.StatusInternalServerError)
			fmt.Printf("Failed to write page: %v\n", err)
			return
		}
	}
}
