package document

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-pdfkit/extract"
	"github.com/go-richdoc/richdoc"
)

// shape describes a document as a line per block, so that a test says what it
// expects in one readable string rather than in a tree of literals.
func shape(blocks []richdoc.Block) string {
	var out []string
	for _, b := range blocks {
		switch n := b.(type) {
		case richdoc.Heading:
			out = append(out, fmt.Sprintf("h%d %q", n.Level, inlines(n.Inlines)))
		case richdoc.Paragraph:
			out = append(out, fmt.Sprintf("p %q", inlines(n.Inlines)))
		case richdoc.List:
			var items []string
			for _, item := range n.Items {
				items = append(items, fmt.Sprintf("%q", itemText(item)))
			}
			out = append(out, "ul "+strings.Join(items, " "))
		case richdoc.Table:
			var rows []string
			rows = append(rows, rowText(n.Header))
			for _, r := range n.Rows {
				rows = append(rows, rowText(r))
			}
			out = append(out, "table "+strings.Join(rows, " / "))
		default:
			out = append(out, fmt.Sprintf("?%T", b))
		}
	}
	return strings.Join(out, "\n")
}

func inlines(in []richdoc.Inline) string {
	var out strings.Builder
	for _, i := range in {
		if t, ok := i.(richdoc.Text); ok {
			out.WriteString(t.Value)
		}
	}
	return out.String()
}

func itemText(item richdoc.ListItem) string {
	if len(item.Blocks) == 0 {
		return ""
	}
	p, ok := item.Blocks[0].(richdoc.Paragraph)
	if !ok {
		return ""
	}
	return inlines(p.Inlines)
}

func rowText(cells []richdoc.Cell) string {
	var out []string
	for _, c := range cells {
		out = append(out, inlines(c.Inlines))
	}
	return "[" + strings.Join(out, "|") + "]"
}

// line writes one line of a content stream at a size and a place.
func line(size int, x, y int, text string) string {
	return fmt.Sprintf("BT /F1 %d Tf %d %d Td (%s) Tj ET", size, x, y, text)
}

// body is n lines of ten-point prose starting at y and running down the page,
// a line apart.
//
// ⛔ A fixture needs REAL proportions. Whether a size is a heading size is
// decided by the share of the document's characters it holds, so a document of
// one heading and one short line makes that heading a fifth of the text — and
// the rule, correctly, calls a fifth of a document body text. Two tests failed
// on that before this helper existed, and the fixtures were what was wrong.
func body(n, y int) string {
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, line(10, 20, y-12*i,
			"a line of ordinary body text with a reasonable number of words on it"))
	}
	return strings.Join(out, "\n")
}

func TestAHeadingIsTextSetLargerThanTheBody(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		line(20, 20, 360, "The Title"),
		line(10, 20, 330, "The body of the document says something here."),
		line(10, 20, 318, "and it goes on for a second line of prose."),
		line(14, 20, 290, "A Section"),
		line(10, 20, 260, "with its own paragraph under it."),
	}, "\n"))
	doc := Parse(d)
	want := strings.Join([]string{
		`h1 "The Title"`,
		`p "The body of the document says something here. and it goes on for a second line of prose."`,
		`h2 "A Section"`,
		`p "with its own paragraph under it."`,
	}, "\n")
	if got := shape(doc.Blocks); got != want {
		t.Errorf("read\n%s\nwant\n%s", got, want)
	}
}

// ⛔ THE CONTROL. A document of nothing but one size has no headings. A rule
// that measures "large" against anything other than the document's own body
// text turns every page of prose into a pile of headings.
func TestADocumentOfOneSizeHasNoHeadings(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		line(11, 20, 360, "One line of prose."),
		line(11, 20, 348, "And a second."),
	}, "\n"))
	doc := Parse(d)
	if got, want := shape(doc.Blocks), `p "One line of prose. And a second."`; got != want {
		t.Errorf("read\n%s\nwant\n%s", got, want)
	}
}

// The body size is the one the most CHARACTERS are drawn at, which is what
// makes a two-line heading over a short paragraph come out the right way
// round: the heading has more lines, the paragraph has more letters.
func TestTheBodyIsCountedInCharactersAndNotInLines(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		line(20, 20, 380, "A Heading"),
		line(20, 20, 356, "On Two Lines"),
		body(6, 320),
	}, "\n"))
	doc := Parse(d)
	got := shape(doc.Blocks)
	// Two lines of heading against six of body: the heading wins on lines and
	// loses on characters, and characters is what decides.
	if !strings.HasPrefix(got, `h1 "A Heading"`) {
		t.Errorf("the heading was not read as one:\n%s", got)
	}
	if !strings.Contains(got, `p "a line of ordinary body text`) {
		t.Errorf("the body was not read as a paragraph:\n%s", got)
	}
}

// Sizes above the body become levels largest-first, however many there are.
func TestEverySizeAboveTheBodyIsALevel(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		line(24, 20, 380, "Biggest"),
		line(18, 20, 350, "Middle"),
		line(14, 20, 320, "Smallest heading"),
		body(8, 290),
	}, "\n"))
	doc := Parse(d)
	got := shape(doc.Blocks)
	for _, want := range []string{`h1 "Biggest"`, `h2 "Middle"`, `h3 "Smallest heading"`} {
		if !strings.Contains(got, want) {
			t.Errorf("%s is missing:\n%s", want, got)
		}
	}
}

// A size above the last level a format has is read AS that level, not dropped:
// text set large is a heading whatever else is on the page.
func TestASizeAboveTheLastLevelIsStillAHeading(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		line(30, 20, 380, "One"), line(28, 20, 360, "Two"), line(26, 20, 340, "Three"),
		line(10, 20, 320, "the body text, which is most of the characters on this page here"),
	}, "\n"))
	doc := ParseWithLimits(d, Limits{HeadingLevels: 2})
	got := shape(doc.Blocks)
	for _, want := range []string{`h1 "One"`, `h2 "Two"`, `h2 "Three"`} {
		if !strings.Contains(got, want) {
			t.Errorf("%s is missing:\n%s", want, got)
		}
	}
}

func TestAGapDownThePageStartsANewParagraph(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		line(10, 20, 360, "The first paragraph begins here"),
		line(10, 20, 348, "and continues on the next line."),
		// Thirty points below a ten-point line: past 1.8 of it.
		line(10, 20, 318, "The second paragraph is its own."),
	}, "\n"))
	doc := Parse(d)
	want := strings.Join([]string{
		`p "The first paragraph begins here and continues on the next line."`,
		`p "The second paragraph is its own."`,
	}, "\n")
	if got := shape(doc.Blocks); got != want {
		t.Errorf("read\n%s\nwant\n%s", got, want)
	}
}

func TestHowFarIsFarIsSaidOutLoud(t *testing.T) {
	content := strings.Join([]string{
		line(10, 20, 360, "One."),
		line(10, 20, 338, "Two."),
	}, "\n")
	d := pagesWith(t, content)
	// Twenty-two points below a ten-point line is 2.2 of it: one paragraph at
	// a break of 3, two at the default of 1.8.
	loose := ParseWithLimits(d, Limits{ParagraphBreak: 3})
	if got := shape(loose.Blocks); got != `p "One. Two."` {
		t.Errorf("at a break of 3 it read\n%s", got)
	}
	tight := Parse(d)
	if got := shape(tight.Blocks); got != "p \"One.\"\np \"Two.\"" {
		t.Errorf("at the default it read\n%s", got)
	}
}

func TestABulletedListIsAList(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		line(10, 20, 360, "What follows is a list of the things to do today ok"),
		"BT /F1 10 Tf 20 330 Td (\225) Tj 20 0 Td (conduct the project) Tj ET",
		"BT /F1 10 Tf 20 318 Td (\225) Tj 20 0 Td (inform the trustees) Tj ET",
		"BT /F1 10 Tf 20 306 Td (\225) Tj 20 0 Td (federate the staff) Tj ET",
	}, "\n"))
	doc := Parse(d)
	want := "p \"What follows is a list of the things to do today ok\"\n" +
		`ul "conduct the project" "inform the trustees" "federate the staff"`
	if got := shape(doc.Blocks); got != want {
		t.Errorf("read\n%s\nwant\n%s", got, want)
	}
}

// A bullet's text wraps, and the line it wraps onto carries no bullet. It is
// the rest of that item and not a paragraph of its own.
func TestAWrappedBulletIsTheRestOfTheItem(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		"BT /F1 10 Tf 20 360 Td (\225) Tj 20 0 Td (conduct the project) Tj ET",
		line(10, 40, 348, "and animate it"),
		"BT /F1 10 Tf 20 330 Td (\225) Tj 20 0 Td (inform the trustees) Tj ET",
	}, "\n"))
	doc := Parse(d)
	want := `ul "conduct the project and animate it" "inform the trustees"`
	if got := shape(doc.Blocks); got != want {
		t.Errorf("read\n%s\nwant\n%s", got, want)
	}
}

// A different mark is a different list.
func TestADifferentMarkIsADifferentList(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		"BT /F1 10 Tf 20 360 Td (\225) Tj 20 0 Td (one) Tj ET",
		"BT /F1 10 Tf 20 348 Td (\225) Tj 20 0 Td (two) Tj ET",
		"BT /F1 10 Tf 20 336 Td (-) Tj 20 0 Td (three) Tj ET",
		"BT /F1 10 Tf 20 324 Td (-) Tj 20 0 Td (four) Tj ET",
	}, "\n"))
	doc := Parse(d)
	want := "ul \"one\" \"two\"\nul \"three\" \"four\""
	if got := shape(doc.Blocks); got != want {
		t.Errorf("read\n%s\nwant\n%s", got, want)
	}
}

// ⛔ A number is not a mark, and a numbered list laid out as "number here,
// text there" is indistinguishable from a two-column table of numbered rows.
// It is read as the table, which keeps BOTH the number and the text in cells
// of their own — lossless, and the alternative is deciding that "1990" at the
// start of a line is a list marker when a table of dates is nothing but those.
//
// Said here rather than in the package comment alone, because somebody will
// eventually read this output and wonder where their list went.
func TestANumberedListIsReadAsATableAndKeepsItsNumbers(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		"BT /F1 10 Tf 20 360 Td (1.) Tj 20 0 Td (the first point) Tj ET",
		"BT /F1 10 Tf 20 348 Td (2.) Tj 20 0 Td (the second point) Tj ET",
	}, "\n"))
	doc := Parse(d)
	want := "table [1.|the first point] / [2.|the second point]"
	if got := shape(doc.Blocks); got != want {
		t.Errorf("read\n%s\nwant\n%s", got, want)
	}
}

// A mark with nothing after it is a line with one piece of text on it, and a
// line of one piece cannot be a marked item.
func TestAMarkOnItsOwnIsNotAnItem(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		"BT /F1 10 Tf 20 360 Td (\225) Tj ET",
		line(10, 20, 348, "the text under it"),
	}, "\n"))
	doc := Parse(d)
	if got := shape(doc.Blocks); strings.Contains(got, "ul ") {
		t.Errorf("a lone mark became a list:\n%s", got)
	}
}

// A long first piece is a word, not a mark.
func TestALongFirstPieceIsAWord(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		"BT /F1 10 Tf 20 360 Td (---) Tj 20 0 Td (not a bullet) Tj ET",
		"BT /F1 10 Tf 20 348 Td (---) Tj 20 0 Td (nor is this) Tj ET",
	}, "\n"))
	doc := Parse(d)
	if got := shape(doc.Blocks); strings.Contains(got, "ul ") {
		t.Errorf("a three-character piece became a mark:\n%s", got)
	}
}

func TestATableIsATableAndIsEmittedOnce(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		line(10, 20, 360, "a paragraph before the grid with enough words to be the body"),
		"BT /F1 10 Tf 20 330 Td (Name) Tj 60 0 Td (Age) Tj ET",
		"BT /F1 10 Tf 20 318 Td (Ada) Tj 60 0 Td (36) Tj ET",
		"BT /F1 10 Tf 20 306 Td (Bob) Tj 60 0 Td (41) Tj ET",
		line(10, 20, 270, "and a paragraph after it."),
	}, "\n"))
	doc := Parse(d)
	want := strings.Join([]string{
		`p "a paragraph before the grid with enough words to be the body"`,
		`table [Name|Age] / [Ada|36] / [Bob|41]`,
		`p "and a paragraph after it."`,
	}, "\n")
	if got := shape(doc.Blocks); got != want {
		t.Errorf("read\n%s\nwant\n%s", got, want)
	}
}

// Two tables on one page are two tables, which is what says the emitted-once
// rule remembers WHICH table rather than only that there was one.
func TestTwoTablesOnOnePage(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		"BT /F1 10 Tf 20 360 Td (Name) Tj 60 0 Td (Age) Tj ET",
		"BT /F1 10 Tf 20 348 Td (Ada) Tj 60 0 Td (36) Tj ET",
		line(10, 20, 320, "a paragraph between them with plenty of letters in it to be the body"),
		"BT /F1 10 Tf 20 290 Td (City) Tj 60 0 Td (Code) Tj ET",
		"BT /F1 10 Tf 20 278 Td (Caen) Tj 60 0 Td (14000) Tj ET",
	}, "\n"))
	doc := Parse(d)
	got := shape(doc.Blocks)
	if n := strings.Count(got, "table "); n != 2 {
		t.Errorf("%d tables, want 2:\n%s", n, got)
	}
	if !strings.Contains(got, "table [City|Code] / [Caen|14000]") {
		t.Errorf("the second table is missing:\n%s", got)
	}
}

// A cell the row left empty is an empty cell, and not a cell holding an empty
// piece of text: a format that writes richdoc out has to be able to tell "no
// content" from "content that says nothing".
func TestACellTheRowLeftEmpty(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		"BT /F1 10 Tf 20 360 Td (Name) Tj 60 0 Td (Age) Tj 60 0 Td (City) Tj ET",
		"BT /F1 10 Tf 20 348 Td (Ada) Tj 120 0 Td (Caen) Tj ET",
	}, "\n"))
	doc := Parse(d)
	if got, want := shape(doc.Blocks), "table [Name|Age|City] / [Ada||Caen]"; got != want {
		t.Errorf("read\n%s\nwant\n%s", got, want)
	}
	table, ok := doc.Blocks[0].(richdoc.Table)
	if !ok {
		t.Fatalf("the block is a %T", doc.Blocks[0])
	}
	if in := table.Rows[0][1].Inlines; in != nil {
		t.Errorf("the empty cell holds %#v, want nothing at all", in)
	}
}

// ⛔ A line is as tall as its TALLEST piece, not as its first. A heading whose
// first character is a small bullet or a footnote mark is still a heading, and
// reading the first piece would make it body text.
func TestALineIsTheSizeMostOfItIsSetAt(t *testing.T) {
	// A heading with a small footnote mark on it is still a heading: most of
	// its characters are the heading.
	d := pagesWith(t, strings.Join([]string{
		"BT /F1 6 Tf 20 380 Td (*) Tj ET BT /F1 20 Tf 40 380 Td (The Title) Tj ET",
		body(6, 340),
	}, "\n"))
	if got := shape(Parse(d).Blocks); !strings.Contains(got, `h1 "* The Title"`) {
		t.Errorf("the heading was read as body text:\n%s", got)
	}

	// ⛔ And the other way, which is why it is MOST and not TALLEST: a
	// paragraph with one large glyph in it is a paragraph. Reading the tallest
	// piece makes it a heading, and a document with one oversized initial
	// loses a paragraph for every one of them.
	d = pagesWith(t, strings.Join([]string{
		line(20, 20, 380, "The Title"),
		"BT /F1 10 Tf 20 340 Td (a line of body text with one large glyph) Tj ET" +
			" BT /F1 20 Tf 240 340 Td (Q) Tj ET",
		body(5, 320),
	}, "\n"))
	got := shape(Parse(d).Blocks)
	if strings.Contains(got, `h1 "a line of body text`) {
		t.Errorf("one large glyph turned a paragraph into a heading:\n%s", got)
	}
	if !strings.Contains(got, "a line of body text with one large glyph Q") {
		t.Errorf("the line is not in a paragraph:\n%s", got)
	}
}

// ⛔ A short LETTER at the start of a line is not a mark. "A something" is a
// sentence, and a list made of it would be a list of sentences with their
// first letters taken away.
func TestAShortLetterIsNotAMark(t *testing.T) {
	d := pagesWith(t, "BT /F1 10 Tf 20 360 Td (A) Tj 20 0 Td (sentence beginning with one letter) Tj ET")
	doc := Parse(d)
	if got, want := shape(doc.Blocks), `p "A sentence beginning with one letter"`; got != want {
		t.Errorf("read\n%s\nwant\n%s", got, want)
	}
}

// ⛔ THE DEFECT THAT DECIDED THE ORDER OF THE WHOLE THING. A numbered heading
// leaves a gap after its number; the table below it leaves that same gap empty
// on every row; so asking the page for its tables returns ONE two-column table
// that is the heading and the grid together — and the grid's real columns are
// gone, because the heading's own text reaches across all of them.
//
// Measured on a 6-page paper: page 2's four-column comparison came back as
// `| 2. | Vue d'ensemble technique |` with everything crushed into the second
// column. The fix is that the tables are looked for among the lines that are
// NOT headings, which is something only this package can know.
func TestANumberedHeadingAboveATableDoesNotSwallowIt(t *testing.T) {
	// ⛔ The fixture has to REPRODUCE it. The first version of this test put
	// the heading's text six points after its number — too narrow to be a
	// boundary at all — so the heading never joined the table and the test
	// passed against the unfixed code. The number is at 20, its text at 60
	// where the grid's own first column starts, and the gap between them is
	// the boundary the grid leaves empty on every row.
	content := strings.Join([]string{
		"BT /F1 14 Tf 20 380 Td (2.) Tj 40 0 Td (Vue d'ensemble technique) Tj ET",
		"BT /F1 10 Tf 60 350 Td (Name) Tj 60 0 Td (Age) Tj 60 0 Td (City) Tj ET",
		"BT /F1 10 Tf 60 338 Td (Ada) Tj 60 0 Td (36) Tj 60 0 Td (Caen) Tj ET",
		"BT /F1 10 Tf 60 326 Td (Bob) Tj 60 0 Td (41) Tj 60 0 Td (Lyon) Tj ET",
		body(6, 290),
	}, "\n")
	d := pagesWith(t, content)

	// The control: asked about the WHOLE page, the heading swallows the grid.
	// This is the defect, asserted so that the fix below is shown to be one.
	runs, err := extract.Runs(d, 1)
	if err != nil {
		t.Fatal(err)
	}
	swallowed := extract.TablesInRuns(runs, 1, extract.TableOptions{})
	if len(swallowed) != 1 || len(swallowed[0].Rows[0]) != 2 {
		t.Fatalf("the fixture does not reproduce the defect: %+v", swallowed)
	}

	doc := Parse(d)
	got := shape(doc.Blocks)
	if !strings.Contains(got, `h1 "2. Vue d'ensemble technique"`) {
		t.Errorf("the heading is not a heading:\n%s", got)
	}
	if !strings.Contains(got, "table [Name|Age|City] / [Ada|36|Caen] / [Bob|41|Lyon]") {
		t.Errorf("the three-column table did not survive the heading:\n%s", got)
	}
}

func TestPagesKeepTheirNumbers(t *testing.T) {
	d := pagesWith(t,
		line(10, 20, 360, "the first page, with most of the characters on it here"),
		line(10, 20, 360, "the second page"))
	pages := Pages(d)
	if len(pages) != 2 {
		t.Fatalf("%d pages", len(pages))
	}
	if pages[0].Number != 1 || pages[1].Number != 2 {
		t.Errorf("the pages say they are %d and %d", pages[0].Number, pages[1].Number)
	}
	if got := shape(pages[1].Blocks); got != `p "the second page"` {
		t.Errorf("page two reads %s", got)
	}
}

// ⛔ The body size is a property of the DOCUMENT and not of a page. A page
// holding nothing but a title would otherwise have that title read as its own
// body text — and a caller asking for one page would get different headings
// from a caller asking for the lot.
func TestTheBodySizeIsTheDocumentsAndNotThePages(t *testing.T) {
	d := pagesWith(t,
		line(20, 20, 360, "A Title Page"),
		strings.Join([]string{
			line(10, 20, 360, "page two is all body text and there is a great deal of it"),
			line(10, 20, 348, "running on for several lines so that it wins the count"),
		}, "\n"))
	pages := Pages(d)
	if got := shape(pages[0].Blocks); got != `h1 "A Title Page"` {
		t.Errorf("the title page read %s", got)
	}
}

// A page that cannot be read contributes no blocks rather than stopping the
// document — which is also why there is no error to return.
func TestAPageThatCannotBeReadIsEmptyAndNotAnError(t *testing.T) {
	doc := Parse(brokenPage(t))
	if len(doc.Blocks) != 0 {
		t.Errorf("it read %d blocks off a page it could not read", len(doc.Blocks))
	}
	// And the page is still THERE, as an empty one: a caller walking pages
	// alongside page numbers must not have them shift under it.
	pages := Pages(brokenPage(t))
	if len(pages) != 1 || len(pages[0].Blocks) != 0 || pages[0].Number != 1 {
		t.Errorf("the page is not there as an empty one: %+v", pages)
	}
}

func TestADocumentWithNoPages(t *testing.T) {
	doc := Parse(pagesWith(t))
	if len(doc.Blocks) != 0 {
		t.Errorf("%d blocks off no pages", len(doc.Blocks))
	}
}

// Text set at no size at all says nothing about how far apart its lines should
// be, and nothing about what "larger than the body" means either.
func TestTextSetAtNoSizeAtAll(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		"BT /F1 0 Tf 20 360 Td (one) Tj ET",
		"BT /F1 0 Tf 20 200 Td (two) Tj ET",
	}, "\n"))
	doc := Parse(d)
	// One paragraph: with no size there is no distance to be far across.
	if got := shape(doc.Blocks); got != `p "one two"` {
		t.Errorf("read %s", got)
	}
}

// A line of nothing but spaces says nothing, and does not close a paragraph:
// it is whitespace drawn with text, and a typesetter emits it freely.
func TestALineOfSpacesSaysNothing(t *testing.T) {
	d := pagesWith(t, strings.Join([]string{
		line(10, 20, 360, "the first line"),
		line(10, 20, 354, "   "),
		line(10, 20, 348, "the second line"),
	}, "\n"))
	doc := Parse(d)
	if got := shape(doc.Blocks); got != `p "the first line the second line"` {
		t.Errorf("read %s", got)
	}
}

// ⛔ THE DEFECT THAT NEEDED THE SHARE RULE. A size above the body that holds a
// third of the document's characters is a SECOND BODY SIZE, not a heading. A
// nine-page report set in 10, 12 and 14 point came back with 131 headings, 130
// of which were paragraphs of prose.
func TestASecondBodySizeIsNotAHeading(t *testing.T) {
	var lines []string
	for i := 0; i < 6; i++ {
		lines = append(lines, line(14, 20, 380-16*i,
			"a line of prose set in fourteen point, of which there is a great deal"))
	}
	lines = append(lines, line(20, 20, 270, "The Title"))
	for i := 0; i < 6; i++ {
		lines = append(lines, line(10, 20, 240-12*i,
			"a line of prose set in ten point, of which there is also a great deal"))
	}
	got := shape(Parse(pagesWith(t, strings.Join(lines, "\n"))).Blocks)
	// The whole shape, not just "the title is a heading": counting only the
	// title lets the fourteen-point prose become h2 unnoticed, which is the
	// defect itself with one more level on it.
	want := strings.Join([]string{
		`p "` + strings.Repeat("a line of prose set in fourteen point, of which there is a great deal ", 5) +
			`a line of prose set in fourteen point, of which there is a great deal"`,
		`h1 "The Title"`,
		`p "` + strings.Repeat("a line of prose set in ten point, of which there is also a great deal ", 5) +
			`a line of prose set in ten point, of which there is also a great deal"`,
	}, "\n")
	if got != want {
		t.Errorf("read\n%s\nwant\n%s", got, want)
	}
}

// ⛔ The body size is the size the most CHARACTERS are at, and that is not the
// size on the most LINES. Seven short heading lines against five long
// paragraphs: counting lines makes the headings into the body and loses both.
func TestManyShortHeadingsDoNotOutvoteFewLongParagraphs(t *testing.T) {
	var lines []string
	for i := 0; i < 7; i++ {
		lines = append(lines, line(20, 20, 380-26*i, "Short"))
	}
	for i := 0; i < 5; i++ {
		lines = append(lines, line(10, 20, 190-12*i,
			"a line of ordinary body text with a reasonable number of words on it"))
	}
	got := shape(Parse(pagesWith(t, strings.Join(lines, "\n"))).Blocks)
	if n := strings.Count(got, `h1 "Short"`); n != 7 {
		t.Errorf("%d of the seven short lines are headings:\n%s", n, got)
	}
}

// Two sizes a fraction of a point apart are the same size to a reader, so a
// document that sets a line at 12.4 where its body is 12 does not acquire a
// heading level for it.
func TestASizeAFractionOfAPointAboveTheBodyIsTheBody(t *testing.T) {
	var lines []string
	for i := 0; i < 6; i++ {
		lines = append(lines, line(12, 20, 380-14*i,
			"a line of ordinary body text with a reasonable number of words on it"))
	}
	lines = append(lines,
		"BT /F1 12.4 Tf 20 280 Td (a line a fraction of a point larger) Tj ET")
	got := shape(Parse(pagesWith(t, strings.Join(lines, "\n"))).Blocks)
	if strings.Contains(got, "h1 ") {
		t.Errorf("four tenths of a point made a heading:\n%s", got)
	}
}
