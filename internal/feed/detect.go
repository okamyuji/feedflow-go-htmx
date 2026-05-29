package feed

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// rdfNamespace RDFつまりRSS 1.0の名前空間です。
const rdfNamespace = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"

// atomNamespace Atomの名前空間です。
const atomNamespace = "http://www.w3.org/2005/Atom"

// detectFormat バイト列の最初の開始要素からフィード形式を判別します。
// BOMや空白や処理命令やコメントは読み飛ばし、最初の開始要素のローカル名と名前空間で判別します。
func detectFormat(data []byte) (port.FeedFormat, error) {
	trimmed := bytes.TrimLeft(data, "\ufeff \t\r\n")
	if len(trimmed) == 0 {
		return "", fmt.Errorf("feed: empty input")
	}
	dec := xml.NewDecoder(bytes.NewReader(trimmed))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return "", fmt.Errorf("feed: no start element found")
		}
		if err != nil {
			return "", fmt.Errorf("failed to tokenize xml: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		local := strings.ToLower(start.Name.Local)
		space := start.Name.Space
		switch {
		case local == "rss":
			return port.FormatRSS2, nil
		case local == "feed" && (space == atomNamespace || space == ""):
			return port.FormatAtom, nil
		case local == "rdf" && space == rdfNamespace:
			return port.FormatRDF, nil
		default:
			return "", fmt.Errorf("feed: unrecognized root element %q (ns=%q)", start.Name.Local, space)
		}
	}
}
