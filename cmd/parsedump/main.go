package main

import (
	"bufio"
	"context"
	"encoding/xml"
	"flag"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/samiam2013/wiki4dummies/wiki"
	"golang.org/x/time/rate"
)

func main() {
	wikiDumpPath := flag.String("dump_path", "", "Path to the wiki dump file")
	outputFolder := flag.String("output_folder", "./pages/", "Folder to store the parsed pages")
	resumeLineNum := flag.Uint64("line_number", 0, "Line # to resume from")
	flag.Parse()

	if *wikiDumpPath == "" {
		slog.Error("Please provide the path to the wiki dump file")
		flag.PrintDefaults()
		return
	}

	// listen for signals to stop and output the line number to resume from first
	lineNum := uint64(0)
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		slog.Warn("Caught signal, shutting down")
		slog.Info("Resume with --line_number flag", "line_number", lineNum)
		os.Exit(0)
	}()

	// listen to a channel for pages and call the function to process each page
	pageC := make(chan string, 2)
	go func() {
		limiter := rate.NewLimiter(rate.Every(70*time.Millisecond), 1)
		for {
			page := <-pageC
			_ = limiter.Wait(context.Background())
			go processPage(page, *outputFolder)
		}
	}()

	dumpFH, err := os.OpenFile(*wikiDumpPath, os.O_RDWR, 0664)
	if err != nil {
		slog.Error("Error opening the wiki dump file", "error", err)
		return
	}
	defer func() { _ = dumpFH.Close() }()
	slog.Info("Successfully opened the wiki dump file")

	// pass each page from the file to the channel
	s := bufio.NewScanner(dumpFH)
	s.Buffer(make([]byte, 0, 64*1024), 100*1024*1024)
	page := ""
	pageSection := false
	for s.Scan() {
		lineNum++
		if *resumeLineNum != 0 && lineNum < *resumeLineNum {
			continue
		}
		line := s.Text()
		// slog.Info("Processing line", "line", line)
		if strings.Contains(line, "<page>") {
			pageSection = true
		}
		if strings.Contains(line, "</page>") {
			pageSection = false
			page += line + "\n"
			pageC <- page
			page = ""
		}
		if pageSection {
			page += line + "\n"
		}
	}
	if err := s.Err(); err != nil {
		slog.Error("Resume with --line_number flag", "line_number", lineNum)
		slog.Error("Error scanning the wiki dump file", "error", err)
		return
	}
	slog.Info("Successfully processed the wiki dump file")
}

func processPage(page, outputFolder string) {
	// call the command line tool to parse the page
	if strings.Contains(page, "#REDIRECT") {
		// slog.Info("Skipping the page as it is a redirect page")
		return
	}
	p := wiki.Page{}
	err := xml.Unmarshal([]byte(page), &p)
	if err != nil {
		slog.Error("Error unmarshalling the page", "error", err)
		return
	}
	title := p.Title
	text := p.Revision.Text.Text

	pyScript := "./mediawiki2html.py"
	pagePath := outputFolder + slugify(title) + ".txt"
	pageSourcePath := "./pages/" + slugify(title) + ".source.xml"
	cmd := exec.Command(pyScript)
	cmd.Stdin = strings.NewReader(text)
	output, err := cmd.Output()
	if err != nil {
		slog.Error("Error running the python script", "error", err)
		return
	}
	// slog.Info("got page", "title", title)

	// write the page and source to files
	pageFH, err := os.OpenFile(pagePath, os.O_CREATE|os.O_RDWR, 0664)
	if err != nil {
		slog.Error("Error opening the page file", "error", err)
		return
	}
	defer func() { _ = pageFH.Close() }()
	if _, err = pageFH.Write(output); err != nil {
		slog.Error("Error writing the page to the file", "error", err)
		return
	}
	_ = pageFH.Sync()
	slog.Info("Successfully wrote the page to the file", "file", pagePath)

	// duplication of the above code to write the source to a file
	pageSourceFH, err := os.OpenFile(pageSourcePath, os.O_CREATE|os.O_RDWR, 0664)
	if err != nil {
		slog.Error("Error opening the page file", "error", err)
		return
	}
	defer func() { _ = pageSourceFH.Close() }()
	if _, err = pageSourceFH.WriteString(page); err != nil {
		slog.Error("Error writing the page to the file", "error", err)
		return
	}
	_ = pageSourceFH.Sync()
	slog.Info("Successfully wrote the page to the source file")

}

var reNonAlphaNum = regexp.MustCompile("[^a-zA-Z0-9-]+")

func slugify(title string) string {
	// replace everything except alphabets and numbers with a hyphen
	title = strings.Trim(title, "\"")
	title = strings.ReplaceAll(title, " ", "-")
	title = reNonAlphaNum.ReplaceAllString(title, "")
	return title
}
