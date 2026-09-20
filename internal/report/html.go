package report

import (
	_ "embed"
	"encoding/json"
	"io"
	"strings"
)

//go:embed report.html
var page string

// WriteHTML renders the document as one static page: the JSON embedded in
// a script element and a small inline renderer with the Projects and
// Places lenses, grouped findings, and an evidence drawer. No external
// assets, no network, no server. The JSON is the contract; the page is a
// first reading of it for the designer to replace.
func WriteHTML(w io.Writer, doc *Document) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	// A closing script tag inside the data would end the element early.
	safe := strings.ReplaceAll(string(data), "</", `<\/`)
	_, err = io.WriteString(w, strings.Replace(page, "__DATA__", safe, 1))
	return err
}
