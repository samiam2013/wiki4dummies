package main

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"context"
	"encoding/xml"
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

	"github.com/samiam2013/gowiki"
	"github.com/samiam2013/wiki4dummies/wiki"
	"golang.org/x/time/rate"
)

const pageFileFolder = "pages"
const indexFileFolder = "index"

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
	limiter := rate.NewLimiter(rate.Every(500*time.Millisecond), 1)

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
				slog.Error("Failed to parse page", "error", err)
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

			_ = text // TODO: index the text
			// if err := indexPage(indexPath, title, text); err != nil {
			// 	slog.Error("Failed to index page", "error", err)
			// }

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

// func indexPage(indexPath, savedFilename, title, text string) error {
// 	return fmt.Errorf("Not implemented")
// }

var nonAlphaNum = regexp.MustCompile("[^a-zA-Z0-9]+")

func savePage(savePath, title string, pageBuffer []byte) (string, error) {
	// slug-ify the title:
	// 1. lowercase 2. replace spaces with dashe 3. remove leading and trailing dashes
	title = strings.ToLower(title)
	title = nonAlphaNum.ReplaceAllString(title, "-")
	title = strings.Trim(title, "-")

	if len(title) < 3 {
		title = fmt.Sprintf("%3s", title)
		title = strings.ReplaceAll(title, " ", "_")
	}

	folderPath, err := trieMake(filepath.Join(savePath, pageFileFolder), title)
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

	leadPath := filepath.Join(savePath, pageFileFolder) + string(filepath.Separator)
	relPath := strings.TrimPrefix(filePath, leadPath)
	return relPath, nil
}

// trieMake creates a directory structure for the title with the first two characters
func trieMake(savePath, title string) (string, error) {
	// create the path
	first := string(title[0]) // this might break on emoji
	second := string(title[1])
	path := filepath.Join(savePath, first, second)
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("failed to create parent directories: %w", err)
	}
	return path, nil
}
