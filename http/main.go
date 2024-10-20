package main

import (
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ollama/ollama/api"
	"github.com/samiam2013/wiki4dummies/constants"
	"github.com/samiam2013/wiki4dummies/normalize"
	"github.com/samiam2013/wiki4dummies/wiki"
	"github.com/semantosoph/gowiki"
	"golang.org/x/sync/errgroup"
)

func main() {
	var savePath string
	var ollama bool
	flag.StringVar(&savePath, "save_path", "", "Path to the save index, page files")
	flag.BoolVar(&ollama, "ollama", false, "Use the ollama API to generate summaries")
	flag.Parse()

	if savePath == "" {
		fmt.Println("The save_path arg is required")
		return
	}

	fmt.Println("Initializing w4d server")
	cache := newResultCache()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/index.html")
	})
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./"+r.URL.Path)
	})
	mux.HandleFunc("/search", handleSearch(savePath, cache, ollama))
	mux.HandleFunc("/page/", handlePage(savePath))
	if ollama {
		ollamaClient, err := api.ClientFromEnvironment()
		if err != nil {
			log.Fatal(err)
		}
		mux.HandleFunc("/ai-summary", handleAISummary(cache, ollamaClient))
	}

	err := http.ListenAndServe(":3030", mux)
	fmt.Printf("Server stopped, error: %v\n", err)
}

type SearchPageData struct {
	Query         string
	SearchTime    string
	FilesReturned int
	Results       []SearchResult
	UseOllama     bool
	CacheKey      string // used for AI generated answers
}

type SearchResult struct {
	Title    string
	URL      string
	Snippet  string
	Abstract string // used for AI generated answers
}

func handleSearch(savePath string, cache *resultCache, useOllama bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if strings.TrimSpace(q) == "" {
			http.Error(w, "No query provided", http.StatusBadRequest)
			return
		}

		data, err := search(savePath, q)
		if err != nil {
			http.Error(w, "Failed to search", http.StatusInternalServerError)
			fmt.Printf("Failed to search: %v\n", err)
			return
		}

		if useOllama {
			data.UseOllama = true
			uuid := uuid.New().String()
			data.CacheKey = uuid
			cache.set(uuid, data, 5*time.Minute)
		}

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

func asciiStringToInt(s string) int {
	result := 0
	for _, r := range s {
		// Subtract '0' (ASCII 48) from the rune value to get the corresponding integer
		result = result*10 + int(r-'0')
	}
	return result
}

func loadIndex(idxPath string) (index, error) {
	f, err := os.Open(idxPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open index file: %w", err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	rows := make([]idxRow, 0, 300_000)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read line: %w", err)
		}

		// Split the line by commas
		parts := strings.Split(line, ",")
		if len(parts) != 3 {
			fmt.Println("Invalid line format:", line)
			continue
		}

		// Parse the fields
		wordFreq := asciiStringToInt(parts[0])
		exactMatch := (parts[1] == "true")
		relPath := strings.TrimSpace(parts[2])

		rows = append(rows, idxRow{WordFreq: wordFreq, ExactMatch: exactMatch, RelPath: relPath})
	}

	return rows, nil
}

const ExactMatchMultiplier = 3

func search(savePath, q string) (SearchPageData, error) {
	fmt.Printf("Searching for: %s\n", q)
	startTime := time.Now()
	// Search the index
	indexPath := filepath.Join(savePath, constants.IndexFileFolder)
	wordFreqs, err := wiki.GatherWordFrequency(strings.NewReader(q))
	if err != nil {
		return SearchPageData{}, fmt.Errorf("failed to gather word frequency: %w", err)
	}
	// TODO: evaluate the usefulness of stemming the search terms
	// stemmedQueryWords := normalize.StemmedWordFreqs(wordFreqs)

	loadIndexStart := time.Now()
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
	fmt.Printf("Loaded indexes in %s\n", time.Since(loadIndexStart).String())

	sortSliceTime := time.Now()
	// sort the pages into a list of matches by index score
	pagesByNumMatches := map[int][]string{}
	maxScore := 0
	for relPath, score := range pages {
		pagesByNumMatches[score] = append(pagesByNumMatches[score], relPath)
		if score > maxScore {
			maxScore = score
		}
	}
	fmt.Printf("Sorted pages in %s\n", time.Since(sortSliceTime).String())

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
	type syncMatchList struct {
		mutex   sync.Mutex
		matches []match
	}
	// for each page, load the page file and search for the query
	startScorePages := time.Now()
	syncList := syncMatchList{mutex: sync.Mutex{}, matches: []match{}}
	eg := errgroup.Group{} // TODO don't need this
	for relPath, score := range pages {
		// TODO this is kludgy, should be removed earlier or something
		if !slices.Contains(topResults, relPath) {
			continue
		}
		eg.Go(func(relPath string, score int) func() error {
			return func() error {
				var m match
				m.relPath = relPath
				m.indexScore = score
				pagePath := filepath.Join(savePath, constants.PageFileFolder, relPath)
				textScore, err := scorePageMatch(pagePath, q)
				if err != nil {
					return fmt.Errorf("failed to score page match: %w", err)
				}
				m.textScore = textScore
				syncList.mutex.Lock()
				syncList.matches = append(syncList.matches, m)
				syncList.mutex.Unlock()
				return nil
			}
		}(relPath, score))
	}
	if err := eg.Wait(); err != nil {
		return SearchPageData{}, fmt.Errorf("failed page search(es): %w", err)
	}

	matchList := syncList.matches
	sort.Slice(matchList, func(i, j int) bool {
		if matchList[i].indexScore+matchList[i].textScore == matchList[j].indexScore+matchList[j].textScore {
			return matchList[i].indexScore > matchList[j].indexScore
		}
		return matchList[i].indexScore+matchList[i].textScore > matchList[j].indexScore+matchList[j].textScore
	})
	fmt.Printf("Scored pages in %s\n", time.Since(startScorePages).String())

	startGetPageData := time.Now()
	spd := SearchPageData{Query: q, Results: []SearchResult{}}
	spd.FilesReturned = len(pages)
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
		sr.Abstract = abstract
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
	fmt.Printf("Got page data in %s\n", time.Since(startGetPageData).String())

	spd.SearchTime = time.Since(startTime).Truncate(10 * time.Millisecond).String()
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
		var wikiPage wiki.Page
		if err := xml.Unmarshal(pageBuffer, &wikiPage); err != nil {
			http.Error(w, "Failed to unmarshal page", http.StatusInternalServerError)
			fmt.Printf("Failed to unmarshal page: %v\n", err)
			return
		}
		// Parse the page
		article, err := gowiki.ParseArticle(wikiPage.Title, wikiPage.Revision.Text.Text, &gowiki.DummyPageGetter{})
		if err != nil {
			http.Error(w, "Failed to parse article", http.StatusInternalServerError)
			fmt.Printf("Failed to parse article: %v\n", err)
			return
		}
		text := article.GetText()
		// limit any number of \n to 2
		newlinesRE := regexp.MustCompile(`\n{3,}`)
		text = newlinesRE.ReplaceAllString(text, "\n\n")
		if _, err := w.Write([]byte(text)); err != nil {
			http.Error(w, "Failed to write article", http.StatusInternalServerError)
			fmt.Printf("Failed to write article: %v\n", err)
			return
		}
	}
}

func handleAISummary(cache *resultCache, ollamaClient *api.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set headers to indicate this is a stream of SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		cacheKey := r.URL.Query().Get("cache_key")
		if cacheKey == "" {
			http.Error(w, "No cache_key provided", http.StatusBadRequest)
			fmt.Printf("Failed to get cache_key from query params\n")
			return
		}

		data, ok := cache.get(cacheKey)
		if !ok {
			http.Error(w, "No data found", http.StatusNotFound)
			fmt.Print("No data found in cache\n")
			return
		}

		var searchSummary string
		for i, result := range data.Results {
			if i >= 5 {
				break
			}
			searchSummary += fmt.Sprintf("%s: %s\n\n\n", result.Title, result.Abstract)
		}
		_ = searchSummary // TODO all for naught?

		req := &api.GenerateRequest{
			Model: "llama3.2:1b",
			Prompt: fmt.Sprintf("You are a helpful search engine assistant. "+
				"Answer this question in a single english sentence: ` %s `", data.Query),
			Options: map[string]any{"num_predict": 300},
		}

		ctx := context.Background()
		respFunc := func(resp api.GenerateResponse) error {
			// Only print the response here; GenerateResponse has a number of other
			// interesting fields you want to examine.
			sanitized := strings.ReplaceAll(string(resp.Response), "\n", "<newline>")
			if _, err := fmt.Fprintf(w, "data: %s\n\n", sanitized); err != nil {
				return err
			}
			flusher, ok := w.(http.Flusher)
			if ok {
				flusher.Flush()
			}
			return nil
		}

		if err := ollamaClient.Generate(ctx, req, respFunc); err != nil {
			http.Error(w, "Failed to generate response", http.StatusInternalServerError)
			fmt.Printf("Failed to generate response: %v\n", err)
			return
		}

		// // end the stream
		// if _, err := fmt.Fprintf(w, "data: %s\n\n", "END"); err != nil {
		// 	http.Error(w, "Failed to write end of response", http.StatusInternalServerError)
		// 	fmt.Printf("Failed to write end of response: %v\n", err)
		// 	return
		// }
	}

}
