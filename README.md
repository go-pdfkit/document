# document

[![CI](https://github.com/go-pdfkit/document/actions/workflows/ci.yml/badge.svg)](https://github.com/go-pdfkit/document/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-pdfkit/document.svg)](https://pkg.go.dev/github.com/go-pdfkit/document)
[![License: BSD-3-Clause](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE)
[![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen.svg)](#how-it-is-checked)

Reads a PDF back as a **document** rather than as text: headings, paragraphs,
lists and tables, into the [richdoc](https://github.com/go-richdoc/richdoc)
model.

```go
doc := document.Parse(src)        // every page, in order
md, _ := markdown.Write(doc)      // and out as Markdown, reST, HTML…

for _, p := range document.Pages(src) {
    // p.Number, p.Blocks — a page at a time, for indexing
}
```

Pure Go, no C. It builds for `GOOS=js/wasm` and every architecture the fleet
targets.

## A PDF has no headings

It has instructions for drawing glyphs at places. Everything a reader sees
above that — this line is a heading, those three are a list, that block is a
table — is an **inference** from where the places are and how big the glyphs
were. So every block this produces is a judgement, not something the file said.

Worth saying plainly, because the usual phrasing — "convert a PDF to Markdown"
— suggests the structure was in there and merely needed a different spelling.
It was not. A PDF is the output of a typesetter, and this is the typesetter's
work read backwards.

There is also **no error to return**. The document has already been parsed to
get this far, and a page that cannot be *read* is an empty page, which is an
answer rather than a failure. An error that is always nil would teach every
caller to write a branch that never runs.

## What a heading is

Text set **larger than the body**, where the body size is the one the document
draws the most *characters* at — measured over every page, because a page
holding nothing but a title would otherwise have that title as its own body
text.

⛔ **But larger is not enough, and taking it as enough was a defect this
reported on four real documents out of six.** A nine-page report set in 10, 12
and 14 point came back with **131 headings, 130 of which were paragraphs of
prose**: the report simply has two body sizes, and "most popular" can only pick
one.

So a heading size also has to be **rare** — which is what a heading is, a line
or two per section against paragraphs per page. Measured over six real
documents, as the share of characters at each size *above* that document's own
body size:

| | |
|---|---|
| heading sizes | 0.01% – **5.76%**. The 5.76% is 14pt across a 213-page reference work: one title line per job description. |
| second body sizes | **32.6%** – 39.4%. A report in 10/12/14 point, and a paper in 9/11. |

The two populations are an order of magnitude apart, and the threshold sits
between them at **15%** with room on both sides. It is not a tuning knob: it
separates "a line or two per section" from "paragraphs per page", and a
document whose heading really is a sixth of its text is a poster.

A line's own size is the size **most of its characters** are set at, not its
tallest piece. A heading with a small footnote mark on it is a heading either
way; a paragraph with one large glyph in it is a *paragraph*, and reading the
tallest piece makes it a heading.

## What the rest are

| | |
|---|---|
| a **paragraph** | consecutive body lines joined, ending at a gap down the page wider than 1.8 of the line's own height. Ordinary leading is about 1.2, and the space a typesetter puts above a paragraph takes its first line past that. |
| a **heading over two lines** | one heading. A heading's text wraps like anything else, and the guard is the same *size* — not the same level, since levels are clamped and thirty, twenty-eight and twenty-six point can share one. |
| a **list** | consecutive lines beginning with the same short mark — one or two characters with no letter or digit in them. A line under an item and *not* marked is the rest of that item: a bullet's text wraps, and the lines it wraps onto carry no bullet. |
| a **table** | what [`extract.Tables`](https://github.com/go-pdfkit/extract#tables) found, which is its own inference and documents its own limits. |

⛔ **A number is not a mark.** Taking `1.` would mean deciding that `1990` at
the start of a line is a list marker, and a table of dates is nothing but
those. A numbered list laid out as "number here, text there" is read as a
two-column **table** instead, which keeps both the number and the text in cells
of their own — lossless, and somebody reading the output should know where
their list went.

## ⛔ The headings are found before the tables, and that order is the whole thing

A numbered heading leaves a gap after its number. The table below it leaves
that same gap empty on every row. So asking the page for its tables returns
**one two-column table that is the heading and the grid together** — and the
grid's real columns are gone, because the heading's own text reaches across all
of them. Measured on a 6-page paper, page 2:

```
| 2. | Vue d’ensemble technique |
| --- | --- |
|  | HGX B300 GH200 NVL32 GB200 NVL36 GB300 NVL72 |
|  | Génération GPU Blackwell Ultra Hopper (GH200) Blackwell (B200) … |
```

Nothing in the alignment says that line is a heading — its **size** says so,
and this package is the only one that knows which sizes those are. So the
tables are looked for among the lines that are *not* headings, through
`extract.TablesInRuns`, and the same page now reads:

```
### 2. Vue d’ensemble technique

| HGX B300 | GH200 NVL32 | GB200 NVL36 | GB300 NVL72 |
| --- | --- | --- | --- |
| Génération GPU Blackwell Ultra | Hopper (GH200) | Blackwell (B200) | … |
```

## What it does not do

Multi-column page layouts, footnotes, ruled lines, and the reading order of a
page whose text was drawn out of order — a PDF is free to draw its last line
first, and nothing here reorders a page beyond putting its lines down the page
and its pieces left to right. Two tables side by side on one page are read as
one, because a table's box says where its words are and not which words.

## How it is checked

100% of statements with no uncovered blocks, and the thresholds are **mutated**
rather than trusted: 27 mutations, every one caught except the tie-break
between two equally popular sizes, which buys determinism and not a different
reading — the code says so where it is.

Then on real documents, which is where every defect above was found, and none
of which a fixture could have shown:

```text
                           pages     h    par    ul/li     tbl
a 213-page reference work    213    618   1257  334/1989    30
a 9-page report                9      1     43    6/21       4
a 6-page technical paper       6     11     29    3/10      12
a 2-page note                  2      6     12    1/3        2
a 3-page prose report          3      0     12    3/6        0
```

The last row is the control: a report with no headings and no tables in it
reports none of either.

## Joining a line to the one above it

⛔ **No space after a hyphen**, because that is where a typesetter broke a
word. `cadre man-` and `ager` joined with a space is `man- ager`, which is not
what the page says; removing the line break says `man-ager`, which is.

⛔ **And the hyphen stays.** Dropping it would read `manager` — right there,
and wrong for `well-known` broken at its own hyphen. The two cannot be told
apart without a dictionary of the language the document is in, and keeping the
hyphen loses nothing a reader cannot undo while dropping it cannot be undone.

## The rest of the family

[**reader**](https://github.com/go-pdfkit/reader) reads and writes the format ·
[**ops**](https://github.com/go-pdfkit/ops) is the verbs and the `pdfops` command ·
[**extract**](https://github.com/go-pdfkit/extract) reads a page back ·
[**render**](https://github.com/go-pdfkit/render) turns a page into pixels ·
[**convert**](https://github.com/go-pdfkit/convert) is the conversions either way ·
[**app**](https://github.com/go-pdfkit/app) is the browser workbench.

Documentation for all of it: <https://go-pdfkit.github.io/docs/>

## Licence

BSD-3-Clause.
