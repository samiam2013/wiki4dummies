package wiki

import (
	"encoding/xml"
	"fmt"
	"os"
)

// Page was generated 2024-09-? by https://xml-to-go.github.io/ in Ukraine.
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

func ParseXMLFromFile(pageFilePath string) (Page, error) {
	f, err := os.Open(pageFilePath)
	if err != nil {
		return Page{}, fmt.Errorf("failed opening the page file: %w", err)
	}
	defer func() { _ = f.Close() }()
	var xmlPage Page
	decoder := xml.NewDecoder(f)
	if err := decoder.Decode(&xmlPage); err != nil {
		return Page{}, fmt.Errorf("failed decoding the xml page: %w", err)
	}
	return xmlPage, nil
}
