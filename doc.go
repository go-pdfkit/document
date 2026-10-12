// Package document reads a PDF back as a document rather than as text.
//
// A PDF page has no headings, no paragraphs and no lists. It has instructions
// for drawing glyphs at places, and everything a reader sees above that is an
// inference from where those places are and how big the glyphs were. So what
// comes out of here is the richdoc model — [richdoc.Heading],
// [richdoc.Paragraph], [richdoc.List], [richdoc.Table] — and every one of
// those is a judgement this package made, not something the file said.
//
// That is worth stating plainly because the usual phrasing, "convert a PDF to
// Markdown", suggests the structure was in there and merely needed a different
// spelling. It was not. A PDF is the output of a typesetter, and this is the
// typesetter's work read backwards.
//
// # What it infers, and from what
//
//	a heading     text set LARGER than the body. The body size is the one the
//	              document uses for the most characters, measured over every
//	              page; sizes above it become heading levels, largest first.
//	a paragraph   consecutive body lines, joined with spaces, ending at a gap
//	              down the page wider than the lines' own leading.
//	a list        consecutive lines that begin with the same short mark.
//	a table       what [extract.Tables] found, which is its own inference and
//	              documents its own limits.
//
// # What it does not do
//
// It does not read ruled lines, columns of a multi-column layout, footnotes,
// or the reading order of a page whose text was drawn out of order — a PDF is
// free to draw its last line first, and nothing here reorders a page beyond
// putting its lines down the page and its pieces left to right.
//
// Nothing here is CGO.
package document
