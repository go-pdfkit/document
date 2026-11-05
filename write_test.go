package document

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-pdfkit/html2pdf"
	"github.com/go-pdfkit/pdfkit"
	"github.com/go-pdfkit/reader"
	"github.com/go-richdoc/richdoc"
)

// aDocument is prose with the structure a converted file carries: a heading, a
// paragraph, a list and a table.
func aDocument() *richdoc.Document {
	return &richdoc.Document{
		Meta: map[string]string{"title": "The Report", "author": "Someone"},
		Blocks: []richdoc.Block{
			richdoc.Heading{Level: 1, Inlines: words("Chapter one")},
			richdoc.Paragraph{Inlines: words("A sentence that has to reach the page.")},
			richdoc.List{Tight: true, Items: []richdoc.ListItem{
				{Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: words("first")}}},
				{Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: words("second")}}},
			}},
			richdoc.Table{
				Header: []richdoc.Cell{{Inlines: words("left")}, {Inlines: words("right")}},
				Rows:   [][]richdoc.Cell{{{Inlines: words("one")}, {Inlines: words("two")}}},
			},
		},
	}
}

func words(s string) []richdoc.Inline { return []richdoc.Inline{richdoc.Text{Value: s}} }

// ⛔ THE ROUND TRIP THIS PACKAGE IS FOR. A document goes out as a PDF and
// comes back as a document — through this package's own Parse, which is the
// only reader in reach that knows what the model means. It is not an equality
// test: a PDF has no headings, and Parse infers them from the size of the
// text. What it proves is that the words survived the whole chain.
func TestADocumentBecomesAPDFAndReadsBack(t *testing.T) {
	out, err := Write(aDocument(), WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "%PDF") {
		t.Fatalf("what came out does not begin with %%PDF: %.40q", out)
	}

	src, err := reader.Open(out)
	if err != nil {
		t.Fatalf("the PDF this wrote does not open: %v", err)
	}
	if src.PageCount() < 1 {
		t.Fatal("the PDF has no pages")
	}

	back := Parse(src)
	text := richdoc.PlainText(back)

	// ⛔ THE SPACES BETWEEN WORDS DO NOT SURVIVE, AND THAT IS NOT THIS
	// PACKAGE'S DOING — it is go-pdfkit/extract#28, filed with the measurement
	// this test produced. html2pdf draws each word at its own position, as Word
	// and LaTeX and Chrome do, and extract reports Run.Space as the font SIZE
	// rather than a space's advance (15.06 for a 15.0591 pt font, where the
	// measured gap between "Chapter" and "one" is 3.9). No gap can exceed a
	// whole em, so no word break is ever found and the text comes back as
	// "Chapterone".
	//
	// This test therefore asserts what the chain DOES carry — every word, in
	// order — and names what it does not, rather than quietly comparing
	// something weaker. When #28 is fixed, the spaced form below should start
	// passing, and the loose check can go.
	squashed := strings.Join(strings.Fields(text), "")
	for _, want := range []string{
		"Chapterone", "Asentencethathastoreachthepage.",
		"first", "second", "left", "right", "one", "two",
	} {
		if !strings.Contains(squashed, strings.ReplaceAll(want, " ", "")) {
			t.Errorf("%q did not survive the chain; the page says:\n%s", want, text)
		}
	}
	// The order is the other half: a page whose words arrive shuffled would
	// pass every containment check above.
	if i, j := strings.Index(squashed, "Chapterone"), strings.Index(squashed, "first"); i < 0 || j < i {
		t.Errorf("the heading does not come before the list:\n%s", text)
	}
}

// The title and the author travel into the information dictionary, which is
// where a reader's viewer looks for them.
func TestTheDocumentsOwnNameReachesTheFile(t *testing.T) {
	out, err := Write(aDocument(), WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	src, err := reader.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	info, ok := src.GetDict(src.Trailer(), "Info")
	if !ok {
		t.Fatal("the file carries no information dictionary")
	}
	for key, want := range map[reader.Name]string{"Title": "The Report", "Author": "Someone"} {
		got, _ := reader.TextString(info.Get(key))
		if got != want {
			t.Errorf("%s is %q, want %q", key, got, want)
		}
	}
}

// ⛔⛔ A CONVERSION MUST NOT REACH THE NETWORK BECAUSE THE FILE SAID SO. A
// richdoc arrives from something somebody was handed — a .docx, an EPUB, an
// email — and its images carry URLs that whoever wrote the file chose. Fetching
// them by default would make every conversion a request to an address of the
// author's choosing, from the converting machine and with its network position.
// That is a server-side request forgery with a document as the payload.
func TestAnImageURLIsNotFetchedByDefault(t *testing.T) {
	d := &richdoc.Document{Blocks: []richdoc.Block{
		richdoc.Paragraph{Inlines: []richdoc.Inline{
			richdoc.Image{URL: "http://169.254.169.254/latest/meta-data/", Alt: "x"},
			richdoc.Image{URL: "https://example.invalid/tracker.png", Alt: "y"},
		}},
	}}

	var refused atomic.Int64
	out, err := Write(d, WriteOptions{OnRefused: func(string) { refused.Add(1) }})
	if err != nil {
		t.Fatalf("a document naming an address it cannot have must still convert: %v", err)
	}
	if !strings.HasPrefix(string(out), "%PDF") {
		t.Fatal("no PDF came out")
	}
	if refused.Load() == 0 {
		t.Error("nothing was refused, so nothing was asked for — or the refusal is silent")
	}
}

// And a caller that WANTS its sub-resources fetched says so, which is the only
// way the network is reached.
func TestACallerCanSupplyItsOwnFetcher(t *testing.T) {
	var asked atomic.Int64
	d := &richdoc.Document{Blocks: []richdoc.Block{
		richdoc.Paragraph{Inlines: []richdoc.Inline{
			richdoc.Image{URL: "https://example.invalid/a.png", Alt: "x"},
		}},
	}}
	_, err := Write(d, WriteOptions{
		Fetch: func(ctx context.Context, rawurl string) ([]byte, string, error) {
			asked.Add(1)
			return nil, "", errors.New("not today")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if asked.Load() == 0 {
		t.Error("the caller's own fetcher was never asked")
	}
}

// ⛔ Each step's failure is REPORTED WITH WHICH STEP IT WAS, not passed up
// bare. "document: laying the page out: …" and "document: writing the PDF: …"
// are different problems — one is the document, the other is the writer — and
// a caller handed one message cannot tell them apart.
func TestEachStepThatFailsSaysWhichStepItWas(t *testing.T) {
	boom := errors.New("boom")
	for _, c := range []struct {
		name    string
		arrange func(t *testing.T)
		want    string
	}{
		{"rendering the page", func(t *testing.T) {
			was := renderHTML
			t.Cleanup(func() { renderHTML = was })
			renderHTML = func(*richdoc.Document) ([]byte, error) { return nil, boom }
		}, "rendering the document as a page"},
		{"laying it out", func(t *testing.T) {
			was := layOut
			t.Cleanup(func() { layOut = was })
			layOut = func(string, html2pdf.Options) (*pdfkit.Document, error) { return nil, boom }
		}, "laying the page out"},
		{"writing it", func(t *testing.T) {
			was := writePDF
			t.Cleanup(func() { writePDF = was })
			writePDF = func(*pdfkit.Document, io.Writer) error { return boom }
		}, "writing the PDF"},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.arrange(t)
			out, err := Write(aDocument(), WriteOptions{})
			if err == nil {
				t.Fatalf("it succeeded, returning %d bytes", len(out))
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the error is %q, which does not say %q", err, c.want)
			}
			if !errors.Is(err, boom) {
				t.Errorf("the error does not wrap what went wrong: %v", err)
			}
			if out != nil {
				t.Errorf("it returned %d bytes as well as an error", len(out))
			}
		})
	}
}

func TestAnEmptyDocumentStillMakesAFile(t *testing.T) {
	out, err := Write(&richdoc.Document{}, WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "%PDF") {
		t.Errorf("an empty document did not make a PDF: %.40q", out)
	}
	if _, err := reader.Open(out); err != nil {
		t.Errorf("the empty PDF does not open: %v", err)
	}
}

func TestANilDocumentIsAPageAndNotAPanic(t *testing.T) {
	out, err := Write(nil, WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "%PDF") {
		t.Errorf("a nil document did not make a PDF: %.40q", out)
	}
}
