package main

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"encoding/xml"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/samiam2013/wiki4dummies/wiki"
	"github.com/semantosoph/gowiki"
)

func main() {
	var wikiDumpPath string
	flag.StringVar(&wikiDumpPath, "dump_path", "", "Path to the Wikipedia dump file")
	flag.Parse()
	if wikiDumpPath == "" {
		slog.Error("The dump_path arg is required")
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

	siteInfo := make([]byte, 0, 1024*1024)
	siteInfoSection := false
	skipPages := false
	pageBuffer := make([]byte, 0, 10*1024*1024)
	pageSection := false
	for s.Scan() {
		line := s.Bytes()
		// TODO comment this out
		// fmt.Println(string(line))
		if bytes.Contains(line, []byte("<siteinfo>")) {
			siteInfoSection = true
			skipPages = false
		}
		if siteInfoSection {
			// line = bytes.ReplaceAll(line, []byte("\000"), []byte(""))
			siteInfo = append(siteInfo, append(line, []byte("\n")...)...)
		}
		if bytes.Contains(line, []byte("</siteinfo>")) {
			siteInfoSection = false
			var si wiki.Siteinfo
			fmt.Printf("siteInfo:\n%s\n", string(siteInfo))
			if err := xml.Unmarshal(siteInfo, &si); err != nil {
				slog.Error("Failed to unmarshal siteinfo", "error", err)
			}
			slog.Info("Parsed siteinfo", "sitename", si.Sitename, "dbname", si.Dbname)
			if si.Dbname != "enwiki" {
				slog.Warn("Skipping non-English Wikipedia", "dbname", si.Dbname)
				skipPages = true
			}
			siteInfo = make([]byte, 0, 1024*1024)
		}
		if skipPages {
			continue
		}

		if bytes.Contains(line, []byte("<page>")) {
			pageSection = true
		}
		if pageSection {
			pageBuffer = append(pageBuffer, append(line, []byte("\n")...)...)
		}
		var page wiki.Page
		if bytes.Contains(line, []byte("</page>")) {
			pageSection = false
			if err := xml.Unmarshal(pageBuffer, &page); err != nil {
				slog.Error("Failed to unmarshal page", "error", err)
			}
			slog.Info("Parsed page", "title", page.Title, "namespace", page.Ns)
			pageBuffer = make([]byte, 0, 10*1024*1024)
		}
		if page.Ns != "0" {
			continue
		}

		article, err := gowiki.ParseArticle(page.Title, page.Revision.Text.Text, &gowiki.DummyPageGetter{})
		if err != nil {
			slog.Error("Failed to parse article", "error", err)
		}
		pageText := article.GetAbstract()
		pageText = strings.Trim(pageText, "\n")
		slog.Info("Successfully parsed page", "title", page.Title, "page", pageText)

	}
	if err := s.Err(); err != nil {
		slog.Error("Failed to scan dump file", "error", err)
	}
	// TODO comment this out after debugging
	// fmt.Println(string(siteInfo))

}
