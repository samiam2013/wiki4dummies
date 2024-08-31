package main

import (
	"flag"
	"log/slog"
	"os"
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

	if _, err := os.Stat(*indexFolder); os.IsNotExist(err) {
		slog.Info("Creating the index folder")
		if err := os.Mkdir(*indexFolder, 0755); err != nil {
			slog.Error("Error creating the index folder", "error", err)
			return
		}
	}

}
