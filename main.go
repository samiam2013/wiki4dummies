package main

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"context"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/samiam2013/wiki4dummies/constants"
	"github.com/samiam2013/wiki4dummies/normalize"
	"github.com/samiam2013/wiki4dummies/wiki"
	"github.com/semantosoph/gowiki"
	"golang.org/x/time/rate"
)

func init() {
	gowiki.DebugLevel = 0 // this should absolutely not be a thing
}

func main() {
	var wikiDumpPath, savePath string
	var resumeLineNum int
	flag.StringVar(&wikiDumpPath, "dump_path", "", "Path to the Wikipedia dump file")
	flag.StringVar(&savePath, "save_path", "", "Path to the save index, page files")
	flag.IntVar(&resumeLineNum, "resume", 0, "Line number to resume from")
	flag.Parse()

	if wikiDumpPath == "" {
		slog.Error("The dump_path arg is required")
	}
	if savePath == "" {
		slog.Error("The save_path arg is required")
	}
	if !strings.HasSuffix(wikiDumpPath, ".xml.bz2") {
		slog.Error("wikiDumpPath must be an bzip2 compressed XML " +
			"'pages-articles-multistream' file")
	}
	slog.Info("Starting stream of Wikipedia dump file", "dump_path", wikiDumpPath)

	fh, err := os.Open(wikiDumpPath)
	if err != nil {
		slog.Error("Failed to open dump file", "error", err)
	}
	defer func() { _ = fh.Close() }()
	xfh := bzip2.NewReader(fh)
	s := bufio.NewScanner(xfh)
	// this is an insane size, unsure this is necessary
	s.Buffer(make([]byte, 0, 64*1024), 100*1024*1024)

	// this section is just to check that it's english wikipedia
	// siteInfoSection := false
	// siteInfo := make([]byte, 0, 1024*1024)
	// for s.Scan() {
	// 	line := s.Bytes()
	// 	if bytes.Contains(line, []byte("<siteinfo>")) {
	// 		siteInfoSection = true
	// 	}
	// 	if siteInfoSection {
	// 		siteInfo = append(siteInfo, append(line, []byte("\n")...)...)
	// 	}
	// 	if bytes.Contains(line, []byte("</siteinfo>")) {
	// 		siteInfoSection = false
	// 		var si wiki.Siteinfo
	// 		fmt.Printf("siteInfo:\n%s\n", string(siteInfo))
	// 		if err := xml.Unmarshal(siteInfo, &si); err != nil {
	// 			slog.Error("Failed to unmarshal siteinfo", "error", err)
	// 			return
	// 		}
	// 		slog.Info("Parsed siteinfo", "sitename", si.Sitename, "dbname", si.Dbname)
	// 		if si.Dbname != "enwiki" {
	// 			slog.Error("Won't parse non-English Wikipedia", "dbname", si.Dbname)
	// 			return
	// 		}
	// 		break
	// 	}
	// }

	// TODO make the rate a const
	limiter := rate.NewLimiter(rate.Every(150*time.Millisecond), 1)

	var lastPageStartLineNum int

	lastLine := func() {
		slog.Info("Last page start line number (use --resume)", "line_num", lastPageStartLineNum)
	}

	// open a channel to listen for signals to stop
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		slog.Info("Received signal to stop")
		lastLine()
		os.Exit(0)
	}()

	pageSection := false
	pageBuffer := make([]byte, 0, 10*1024*1024)
	lineNum := 0
	for s.Scan() {
		line := s.Bytes()
		lineNum++
		if lineNum < resumeLineNum {
			continue
		}
		if bytes.Contains(line, []byte("<page>")) {
			lastPageStartLineNum = lineNum - 1
			pageSection = true
		}
		if pageSection {
			pageBuffer = append(pageBuffer, append(line, []byte("\n")...)...)
		}
		if bytes.Contains(line, []byte("</page>")) {

			pageSection = false
			pageCopy := append([]byte(nil), pageBuffer...)
			title, abstract, text, err := parsePage(pageBuffer)
			if err != nil {
				if !errors.Is(err, ErrNonArticlePage) {
					slog.Error("Failed to parse page", "error", err)
				}
				pageSection = false
				pageBuffer = make([]byte, 0, 10*1024*1024)
				continue
			}
			// slog.Info("Parsed page", "title", title)

			if err := limiter.Wait(context.Background()); err != nil {
				slog.Error("Failed to wait for limiter", "error", err)
				lastLine()
				return
			}

			// coalesce abstract and text
			if text == "" {
				text = abstract
			}

			relSavedPath, err := savePage(savePath, title, pageCopy)
			if err != nil {
				slog.Error("Failed to save page", "error", err)
				pageSection = false
				pageBuffer = make([]byte, 0, 10*1024*1024)
				continue
			}
			slog.Info("Saved page", "title", title, "relative path", relSavedPath)

			if err := indexPage(savePath, relSavedPath, text); err != nil {
				slog.Error("Failed to index page", "error", err)
			}

			pageBuffer = make([]byte, 0, 10*1024*1024)
		}
	}
	if err := s.Err(); err != nil {
		slog.Error("Failed to scan dump file", "error", err)
	}

}

var ErrNonArticlePage = fmt.Errorf("skipping non-article page")

// parsePage returns title, abstract, text, error and only contains the text if
// it was not able to parse the abstract
func parsePage(pageBuffer []byte) (string, string, string, error) {
	var page wiki.Page
	if err := xml.Unmarshal(pageBuffer, &page); err != nil {
		return "", "", "", fmt.Errorf("failed to unmarshal page: %w", err)
	}

	if page.Ns != "0" {
		return "", "", "", fmt.Errorf("non-article page: %w type %s title %s",
			ErrNonArticlePage, page.Ns, page.Title)
	}
	if page.Redirect.Title != "" {
		return "", "", "", fmt.Errorf("redirect page: %w title %s", ErrNonArticlePage, page.Title)
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

func indexPage(savePath, relSavedPath, text string) error {
	// build the word frequency
	wordFreqs, err := wiki.GatherWordFrequency(strings.NewReader(text))
	if err != nil {
		return fmt.Errorf("failed to gather word frequency: %w", err)
	}
	// build a copy of the word frequency but stemmed
	stemmedWordFreqs := normalize.StemmedWordFreqs(wordFreqs)

	// build the path for the index
	indexPath := filepath.Join(savePath, constants.IndexFileFolder)
	// add the exact word match freqs to the indexes
	for word, freq := range wordFreqs {
		triePath, err := normalize.TrieMake(indexPath, word)
		if err != nil {
			return fmt.Errorf("failed to make trie path: %w", err)
		}
		idxSavePath := filepath.Join(triePath, word+".idx")
		if err := addToIndex(idxSavePath, freq, true, relSavedPath); err != nil {
			return fmt.Errorf("failed to add to index: %w", err)
		}
	}

	// add the stemmed word match freqs to the indexes
	for word, freq := range stemmedWordFreqs {
		triePath, err := normalize.TrieMake(indexPath, word)
		if err != nil {
			return fmt.Errorf("failed to make trie path: %w", err)
		}
		idxSavePath := filepath.Join(triePath, word+".idx")
		if err := addToIndex(idxSavePath, freq, false, relSavedPath); err != nil {
			return fmt.Errorf("failed to add to index: %w", err)
		}
	}
	return nil
}

func addToIndex(indexPath string, freq int, exact bool, relSavedPath string) error {
	// if the file does not exist, create it
	fh, err := os.OpenFile(indexPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open index file: %w", err)
	}
	defer func() { _ = fh.Close() }()

	line := fmt.Sprintf("%d,%t,%s\n", freq, exact, relSavedPath)
	// always seek to the end of the file first, can't hurt, necessary often
	if _, err := fh.Seek(0, 2); err != nil {
		return fmt.Errorf("failed to seek to end of file: %w", err)
	}
	// write the line
	if _, err := fh.WriteString(line); err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}
	// slog.Info("Added to index", "index_path", indexPath, "line", line)

	return nil
}

var nonAlphaNum = regexp.MustCompile("[^a-zA-Z0-9]+")

func savePage(savePath, title string, pageBuffer []byte) (string, error) {
	// slug-ify the title:
	// 1. lowercase 2. replace spaces with dashe 3. remove leading and trailing dashes
	title = strings.ToLower(title)
	title = nonAlphaNum.ReplaceAllString(title, "-")
	title = strings.Trim(title, "-")

	folderPath, err := normalize.TrieMake(filepath.Join(savePath, constants.PageFileFolder), title)
	if err != nil {
		return "", fmt.Errorf("failed to save page: %w", err)
	}

	filePath := filepath.Join(folderPath, title+".xml")
	fh, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = fh.Close() }()
	if _, err := fh.Write(pageBuffer); err != nil {
		return "", fmt.Errorf("failed to write to file: %w", err)
	}

	leadPath := filepath.Join(savePath, constants.PageFileFolder) + string(filepath.Separator)
	relPath := strings.TrimPrefix(filePath, leadPath)
	return relPath, nil
}
