package document

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-pdfkit/reader"
)

// pagesWith builds a document whose pages carry the content streams given, set
// in a font of even width so that a test can work out where every letter
// lands: every character is half an em, so at ten points a letter is five
// points wide and a space is five points too.
func pagesWith(t *testing.T, contents ...string) *reader.Document {
	t.Helper()
	b := bytesOf(t, contents...)
	d, err := reader.Open(b)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// bytesOf is pagesWith as the file itself, for the tests that need a path.
func bytesOf(t *testing.T, contents ...string) []byte {
	t.Helper()
	w := reader.NewWriter("1.7")
	pagesRef := w.Reserve()
	widths := make(reader.Array, 0, 224)
	for i := 32; i < 256; i++ {
		widths = append(widths, reader.Integer(500))
	}
	font := w.Add(reader.Dict{
		"Type": reader.Name("Font"), "Subtype": reader.Name("Type1"),
		"BaseFont": reader.Name("Helvetica"), "FirstChar": reader.Integer(32),
		"LastChar": reader.Integer(255), "Widths": widths,
		"Encoding": reader.Name("WinAnsiEncoding"),
	})
	kids := make(reader.Array, 0, len(contents))
	for _, content := range contents {
		kids = append(kids, w.Add(reader.Dict{
			"Type": reader.Name("Page"), "Parent": pagesRef,
			"MediaBox": reader.Array{reader.Integer(0), reader.Integer(0),
				reader.Integer(400), reader.Integer(400)},
			"Contents":  w.Add(&reader.Stream{Dict: reader.Dict{}, Raw: []byte(content)}),
			"Resources": reader.Dict{"Font": reader.Dict{"F1": font}},
		}))
	}
	w.Put(pagesRef, reader.Dict{"Type": reader.Name("Pages"),
		"Kids": kids, "Count": reader.Integer(len(kids))})
	root := w.Add(reader.Dict{"Type": reader.Name("Catalog"), "Pages": pagesRef})
	out, err := w.Finish(reader.Dict{"Root": root})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// written is bytesOf saved to a file, for a test that needs one on disk.
func written(t *testing.T, contents ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.pdf")
	if err := os.WriteFile(path, bytesOf(t, contents...), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// brokenPage is a document whose one page cannot be decoded at all: the stream
// says it is deflated and is not.
func brokenPage(t *testing.T) *reader.Document {
	t.Helper()
	w := reader.NewWriter("1.7")
	pagesRef := w.Reserve()
	page := w.Add(reader.Dict{
		"Type": reader.Name("Page"), "Parent": pagesRef,
		"MediaBox": reader.Array{reader.Integer(0), reader.Integer(0),
			reader.Integer(400), reader.Integer(400)},
		"Contents": w.Add(&reader.Stream{
			Dict: reader.Dict{"Filter": reader.Name("FlateDecode")},
			Raw:  []byte("this is not deflated at all"),
		}),
	})
	w.Put(pagesRef, reader.Dict{"Type": reader.Name("Pages"),
		"Kids": reader.Array{page}, "Count": reader.Integer(1)})
	root := w.Add(reader.Dict{"Type": reader.Name("Catalog"), "Pages": pagesRef})
	out, err := w.Finish(reader.Dict{"Root": root})
	if err != nil {
		t.Fatal(err)
	}
	d, err := reader.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
