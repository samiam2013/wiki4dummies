package wiki

import "encoding/xml"

// Siteinfo was generated 2024-09-08 14:09:30 by https://xml-to-go.github.io/ in Ukraine.
type Siteinfo struct {
	XMLName    xml.Name `xml:"siteinfo"`
	Text       string   `xml:",chardata"`
	Sitename   string   `xml:"sitename"`
	Dbname     string   `xml:"dbname"`
	Base       string   `xml:"base"`
	Generator  string   `xml:"generator"`
	Case       string   `xml:"case"`
	Namespaces struct {
		Text      string `xml:",chardata"`
		Namespace []struct {
			Text string `xml:",chardata"`
			Key  string `xml:"key,attr"`
			Case string `xml:"case,attr"`
		} `xml:"namespace"`
	} `xml:"namespaces"`
}
