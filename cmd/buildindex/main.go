package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/samiam2013/wiki4dummies/normalize"
	"github.com/samiam2013/wiki4dummies/wiki"
)

func main() {
	pagesFolder := flag.String("pages_folder", "", "Folder to gather pages from")
	indexFolder := flag.String("index_folder", "", "Folder to store the index")
	resumeFileCount := flag.Int("resume_file_count", 0, "File count to resume from")
	flag.Parse()

	// maybe these shouldn't be pointers?
	if *pagesFolder == "" {
		slog.Error("The -pages_folder argument is required")
	}
	if *indexFolder == "" {
		slog.Error("The -index_folder argument is required")
	}
	if *resumeFileCount < 0 {
		slog.Error("The -resume_file_count argument must be a positive integer")
	}

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

	fileCount := 0
	// listen for sigint or sigterm to close the channel
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		slog.Info("Caught signal, shutting down", "file_count", fileCount)
		os.Exit(1)
	}()

	if err := filepath.Walk(*pagesFolder, func(path string, info os.FileInfo, err error) error {
		fileCount++
		if *resumeFileCount != 0 && fileCount < *resumeFileCount {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed walking the pages folder: %w file_count: %d", err, fileCount)
		}
		if info.IsDir() {
			return nil
		}
		// TODO: if it's a Category, Template or Wikipedia org page, skip it

		parsed, err := wiki.ParseXML(path)
		if err != nil {
			return fmt.Errorf("failed parsing the title and content: %w file_count: %d", err, fileCount)
		}

		// TODO: figure out how to skip non-english content

		fmt.Printf("title: %s\ncontent:\n%s\n\n", title, content)
		return fmt.Errorf("stop early, debugging")

		return nil
	}); err != nil {
		slog.Error("Error walking the pages folder", "error", err)
		return
	}

}

func gatherWordFrequency(r io.Reader) (map[string]int, error) {
	// f, err := os.Open(pageFile)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed opening the page file: %w", err)
	// }
	// defer func() { _ = f.Close() }()
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 100*100*1024)
	wordFreq := make(map[string]int)
	for s.Scan() {
		words := normalize.SplitAndLower(s.Text())
		for _, word := range words {
			wordFreq[word]++
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("failed scanning the page file: %w", err)
	}
	return wordFreq, nil
}
