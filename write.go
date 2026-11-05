package document

import (
	"bytes"
	"fmt"
	"io"

	"github.com/go-pdfkit/html2pdf"
	"github.com/go-pdfkit/pdfkit"
	"github.com/go-richdoc/html"
	"github.com/go-richdoc/richdoc"
)

// WriteOptions says how a document is laid out. The zero value is A4 with a
// 20 mm margin and no network at all.
type WriteOptions struct {
	// PageSize is the paper. The zero value is pdfkit.A4.
	PageSize pdfkit.PageSize

	// MarginMm is the margin on every side. The zero value is 20.
	MarginMm float64

	// Fetch replaces the refusal below for a caller that WANTS sub-resources
	// fetched — an <img src="https://…"> in the document, a stylesheet. It is
	// the whole of the network surface of this function.
	//
	// ⛔ NIL MEANS REFUSE, AND THAT IS THE POINT, not an oversight to be fixed
	// by a caller who forgets. A richdoc arrives from a file somebody was
	// handed: a .docx, an EPUB, an email. Its images carry URLs that whoever
	// wrote the file chose, so fetching them by default would make every
	// conversion a request to an address of the author's choosing, from the
	// converting machine, with its network position — which is a
	// server-side request forgery with a document as the payload.
	//
	// Converting a file must not reach the network because the file said so.
	Fetch html2pdf.Fetcher

	// OnRefused, when set, is told the URL of each sub-resource that was
	// refused, so a caller can say what the document asked for and did not get
	// rather than silently producing a page with holes in it.
	OnRefused func(string)
}

// Write lays a [richdoc.Document] out as a PDF.
//
// ⛔ IT IS THE DIRECTION THAT DID NOT EXIST. This package reads a PDF back as
// a document; everything in the fleet that produces a richdoc — Word, Excel
// and PowerPoint through go-ooxml, ODT, ODS, ODP and ODG through go-odf,
// Markdown, CSV, JSON, XML, plain text, EPUB, MOBI, FB2 and email through
// go-richdoc — had nowhere to go afterwards. Measured on 2026-11-05 with
// fleetverbs: every ToPDF in the fleet was in go-pdfkit/convert, and every one
// of them took pictures, an SVG, a ZIP of pictures or an XPS package.
// Seventeen capabilities were missing for want of this.
//
// # It composes rather than typesets
//
// [github.com/go-richdoc/html.Write] renders the document as a page and
// [github.com/go-pdfkit/html2pdf.Export] lays that page out. Neither half is
// new, and a typesetter written here would be a third implementation of line
// breaking, page breaking and table layout in one organisation.
//
// What that pair cannot do, this cannot do either, and it is worth knowing
// before a document is converted: html2pdf does not run JavaScript or
// @font-face, draws no CSS background images, and renders CJK characters as
// nothing. Its own documentation is the list.
//
// # Metadata
//
// A document's title and author travel into the PDF's information dictionary
// from its Meta, because that is where a reader's viewer looks for them.
// The three steps, as seams a test replaces.
//
// ⛔ Each of them returns an error that cannot be provoked from outside:
// html.Write's is always nil today, Export refuses only what this never
// produces, and the writer here is a bytes.Buffer. Handling them is required
// by their signatures, so they are not speculative guards — but a branch no
// test can enter is a claim nobody checks, and this repository's gate says so
// in statements. Naming them here is how they get entered.
var (
	renderHTML = html.Write
	layOut     = html2pdf.Export
	writePDF   = func(doc *pdfkit.Document, w io.Writer) error { return doc.Write(w) }
)

func Write(d *richdoc.Document, opt WriteOptions) ([]byte, error) {
	page, err := renderHTML(d)
	if err != nil {
		// html.Write's error is always nil today; a converter that stopped
		// checking it would be wrong the day it is not.
		return nil, fmt.Errorf("document: rendering the document as a page: %w", err)
	}

	fetch := opt.Fetch
	if fetch == nil {
		fetch = html2pdf.Offline
	}
	laid, err := layOut(string(page), html2pdf.Options{
		PageSize:  opt.PageSize,
		MarginMm:  opt.MarginMm,
		Title:     metaOf(d, "title"),
		Author:    metaOf(d, "author"),
		Subject:   metaOf(d, "description"),
		Fetch:     fetch,
		OnRefused: opt.OnRefused,
	})
	if err != nil {
		return nil, fmt.Errorf("document: laying the page out: %w", err)
	}

	var out bytes.Buffer
	if err := writePDF(laid, &out); err != nil {
		return nil, fmt.Errorf("document: writing the PDF: %w", err)
	}
	return out.Bytes(), nil
}

// metaOf reads one key of a document's metadata.
func metaOf(d *richdoc.Document, key string) string {
	if d == nil || d.Meta == nil {
		return ""
	}
	return d.Meta[key]
}
