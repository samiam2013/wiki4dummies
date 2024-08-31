package main

import (
	"bufio"
	"encoding/xml"
	"flag"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
)

func main() {
	wikiDumpPath := flag.String("dump_path", "", "Path to the wiki dump file")
	resumeLineNum := flag.Uint64("line_number", 0, "Line # to resume from")
	flag.Parse()

	if *wikiDumpPath == "" {
		slog.Error("Please provide the path to the wiki dump file")
		flag.PrintDefaults()
		return
	}

	dumpFH, err := os.OpenFile(*wikiDumpPath, os.O_RDWR, 0664)
	if err != nil {
		slog.Error("Error opening the wiki dump file", "error", err)
		return
	}
	defer func() { _ = dumpFH.Close() }()
	slog.Info("Successfully opened the wiki dump file")

	lineNum := uint64(0)
	// listen for signals to stop and output the line number to resume from first
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		slog.Warn("Caught signal, shutting down")
		slog.Info("Resume with --line_number flag", "line_number", lineNum)
		os.Exit(0)
	}()

	// make a new scanner to go through the dump file line by line
	s := bufio.NewScanner(dumpFH)
	s.Buffer(make([]byte, 0, 64*1024), 100*1024*1024)
	page := ""
	pageSection := false
	pageC := make(chan string, 2)
	go func() {
		for {
			page := <-pageC
			go processPage(page)
		}
	}()
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

func processPage(page string) {
	// call the command line tool to parse the page
	if strings.Contains(page, "#REDIRECT") {
		// slog.Info("Skipping the page as it is a redirect page")
		return
	}
	p := Page{}
	err := xml.Unmarshal([]byte(page), &p)
	if err != nil {
		slog.Error("Error unmarshalling the page", "error", err)
		return
	}
	title := p.Title
	text := p.Revision.Text.Text

	pyScript := "./mediawiki2html.py"
	pagePath := "./pages/" + slugify(title) + ".txt"
	cmd := exec.Command(pyScript)
	cmd.Stdin = strings.NewReader(text)
	output, err := cmd.Output()
	if err != nil {
		slog.Error("Error running the python script", "error", err)
		return
	}
	// slog.Info("got page", "title", title)

	// write the page to a file
	pageFH, err := os.OpenFile(pagePath, os.O_CREATE|os.O_RDWR, 0664)
	if err != nil {
		slog.Error("Error opening the page file", "error", err)
		return
	}
	defer func() { _ = pageFH.Close() }()
	_, err = pageFH.Write(output)
	if err != nil {
		slog.Error("Error writing the page to the file", "error", err)
		return
	}
	slog.Info("Successfully wrote the page to the file", "title", title)

}

var reNonAlphaNum = regexp.MustCompile("[^a-zA-Z0-9-]+")

func slugify(title string) string {
	// replace everything except alphabets and numbers with a hyphen
	title = strings.Trim(title, "\"")
	title = strings.ReplaceAll(title, " ", "-")
	title = reNonAlphaNum.ReplaceAllString(title, "")
	return title
}

type Page struct {
	XMLName  xml.Name `xml:"page"`
	Text     string   `xml:",chardata"`
	Title    string   `xml:"title"`
	Ns       string   `xml:"ns"`
	ID       string   `xml:"id"`
	Redirect struct {
		Text  string `xml:",chardata"`
		Title string `xml:"title,attr"`
	} `xml:"redirect"`
	Revision struct {
		Chardata    string `xml:",chardata"`
		ID          string `xml:"id"`
		Parentid    string `xml:"parentid"`
		Timestamp   string `xml:"timestamp"`
		Contributor struct {
			Text     string `xml:",chardata"`
			Username string `xml:"username"`
			ID       string `xml:"id"`
		} `xml:"contributor"`
		Comment string `xml:"comment"`
		Origin  string `xml:"origin"`
		Model   string `xml:"model"`
		Format  string `xml:"format"`
		Text    struct {
			Text  string `xml:",chardata"`
			Bytes string `xml:"bytes,attr"`
			Sha1  string `xml:"sha1,attr"`
			Space string `xml:"space,attr"`
		} `xml:"text"`
		Sha1  string `xml:"sha1"`
		Minor string `xml:"minor"`
	} `xml:"revision"`
}
