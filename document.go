package document

import (
	"sort"
	"strings"
	"unicode"

	"github.com/go-pdfkit/extract"
	"github.com/go-pdfkit/reader"
	"github.com/go-richdoc/richdoc"
)

// A Page is one page read back as blocks, with the number it had.
//
// The number is here because a caller indexing a document for retrieval has to
// be able to say which page a passage came from, and because a page is the
// only unit a PDF actually has.
type Page struct {
	Number int
	Blocks []richdoc.Block
}

// Limits bound what the inference will say. A zero field means the default, so
// Limits{} is the default everywhere and a caller changes one thing by naming
// one field.
type Limits struct {
	// HeadingLevels is how many sizes above the body text get to be headings.
	// The default is 6, which is as many as Markdown and HTML have; a size
	// above the sixth is read AS the sixth rather than ignored, because text
	// set large is a heading whatever else is on the page.
	HeadingLevels int

	// ParagraphBreak is how far below the line above a line has to sit,
	// counted in that line's own height, before it starts a new paragraph
	// instead of continuing one. The default is 1.8: ordinary leading is
	// about 1.2, and the space a typesetter puts above a paragraph takes the
	// first line of it past this.
	ParagraphBreak float64
}

const (
	defaultHeadingLevels  = 6
	defaultParagraphBreak = 1.8

	// markRunes is how long a line's first piece can be and still be a list
	// marker rather than a word. One rune covers the bullet characters, two
	// covers a mark somebody drew with two.
	markRunes = 2

	// headingShareLimit is the largest share of a document's characters a
	// size can hold and still be read as a heading size rather than as a
	// second body size.
	//
	// ⛔ MEASURED, over six real documents, as the share of the characters at
	// each size ABOVE that document's body size:
	//
	//	heading sizes       0.01% .. 5.76%    the 5.76% is 14pt across a
	//	                                      213-page reference work, one
	//	                                      title line per job description
	//	second body sizes   32.6% .. 39.4%    a report set in 10/12/14 point,
	//	                                      and a paper in 9/11
	//
	// The two populations are an order of magnitude apart and the threshold
	// sits between them with room on both sides. It is not a tuning knob: it
	// separates "a line or two per section" from "paragraphs per page", and a
	// document whose heading really is a sixth of its text is a poster.
	headingShareLimit = 0.15
)

func (l Limits) withDefaults() Limits {
	if l.HeadingLevels <= 0 {
		l.HeadingLevels = defaultHeadingLevels
	}
	if l.ParagraphBreak <= 0 {
		l.ParagraphBreak = defaultParagraphBreak
	}
	return l
}

// Parse reads the whole document, one page after another.
//
// ⛔ There is no error. Nothing here can fail: the document has already been
// parsed to get this far, and a page that cannot be READ is an empty page,
// which is an answer rather than a failure. Returning an error that is always
// nil would teach every caller to write a branch that never runs — and the one
// thing worse than an unhandled error is a handled one that cannot happen.
func Parse(d *reader.Document) *richdoc.Document {
	return ParseWithLimits(d, Limits{})
}

// ParseWithLimits is [Parse] with the thresholds said out loud.
func ParseWithLimits(d *reader.Document, l Limits) *richdoc.Document {
	out := &richdoc.Document{}
	for _, p := range PagesWithLimits(d, l) {
		out.Blocks = append(out.Blocks, p.Blocks...)
	}
	return out
}

// Pages reads the document a page at a time, keeping the page numbers.
func Pages(d *reader.Document) []Page {
	return PagesWithLimits(d, Limits{})
}

// PagesWithLimits is [Pages] with the thresholds said out loud.
//
// ⛔ Every page is read, even when a caller only wants one, because the body
// text size is a property of the DOCUMENT. A page holding nothing but a title
// would otherwise have that title read as its body text, and a caller asking
// for page nine would get different headings from a caller asking for the lot.
//
// A page that cannot be read contributes no blocks rather than stopping the
// document: half a report is worth more to somebody searching than an error,
// which is the choice [extract] already made for the text. It is also why
// there is no error to return — see [Parse].
func PagesWithLimits(d *reader.Document, l Limits) []Page {
	l = l.withDefaults()
	byPage := make([][]extract.Run, 0, d.PageCount())
	for page := 1; page <= d.PageCount(); page++ {
		runs, err := extract.Runs(d, page)
		if err != nil {
			runs = nil
		}
		byPage = append(byPage, runs)
	}
	levels := headingLevels(byPage, l.HeadingLevels)
	out := make([]Page, 0, len(byPage))
	for i, runs := range byPage {
		page := i + 1
		lines := extract.Lines(runs)
		out = append(out, Page{
			Number: page,
			Blocks: blocksOf(lines, tablesBelowTheHeadings(lines, levels, page), levels, l),
		})
	}
	return out
}

// tablesBelowTheHeadings finds the page's tables among the lines that are NOT
// headings.
//
// ⛔ Asking about the whole page gets the wrong answer, and this is the defect
// that says why. A numbered heading leaves a gap after its number; the table
// below it leaves that same gap empty on every row; so the two come back as
// ONE two-column table, and the real four-column table under it is lost
// because the heading's own text reaches across every boundary of it. Measured
// on a 6-page paper, whose page 2 read as `| 2. | Vue d'ensemble technique |`
// with the whole comparison crushed into the second column.
//
// Nothing in the alignment says that line is a heading. Its SIZE says so, and
// this is the one place that knows which sizes those are.
func tablesBelowTheHeadings(lines [][]extract.Run, levels map[int]int, page int) []extract.Table {
	var rest []extract.Run
	for _, line := range lines {
		if _, isHeading := levels[round(lineSize(line))]; isHeading {
			continue
		}
		rest = append(rest, line...)
	}
	return extract.TablesInRuns(rest, page, extract.TableOptions{})
}

// headingLevels maps a rounded text size to a heading level.
//
// The body size is the one the document draws the most CHARACTERS at, not the
// one it uses on the most lines. That much is PyMuPDF4LLM's IdentifyHeaders
// rule, and it is the right starting point: there is nothing in a PDF that
// says "heading", so a document's own body size is the only thing available
// that says what "large" means here.
//
// ⛔ But being larger than the body is not enough, and taking it as enough is
// a defect this reported on four real documents out of six. A size above the
// body that holds a THIRD of the document's characters is a second body size,
// not a heading — and a nine-page report set in 10, 12 and 14 point came back
// with 131 headings, 130 of which were paragraphs of prose.
//
// So a heading size also has to be RARE, which is what a heading is: a line or
// two per section against paragraphs per page. See [headingShareLimit].
func headingLevels(byPage [][]extract.Run, max int) map[int]int {
	chars, total := map[int]int{}, 0
	for _, runs := range byPage {
		for _, r := range runs {
			if size := round(r.Size); size > 0 {
				n := len([]rune(r.Text))
				chars[size] += n
				total += n
			}
		}
	}
	body, most := 0, 0
	for size, n := range chars {
		// The tie-break buys DETERMINISM and not a different reading, and no
		// test can tell it from a map-order choice: two sizes with exactly
		// equal character counts are each a third of the document or more, so
		// whichever wins, the other is a second body size and is excluded
		// below either way. Without it the same file could parse two ways on
		// two runs, which is worse than either answer.
		if n > most || (n == most && size < body) {
			body, most = size, n
		}
	}
	var above []int
	for size, n := range chars {
		if size > body && float64(n) < headingShareLimit*float64(total) {
			above = append(above, size)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(above)))
	levels := map[int]int{}
	for i, size := range above {
		level := i + 1
		if level > max {
			level = max
		}
		levels[size] = level
	}
	return levels
}

// round is a text size as a whole number of points. Two sizes a hundredth of a
// point apart are the same size to a reader, and a document that sets its body
// text at 11.9999 on one page should not acquire a heading level for it.
func round(size float64) int { return int(size + 0.5) }

// blocksOf turns one page's lines into blocks.
//
// One paragraph or one list is open at a time, and the same three things close
// whichever it is: a heading, a table, and a gap down the page.
func blocksOf(lines [][]extract.Run, tables []extract.Table, levels map[int]int, l Limits) []richdoc.Block {
	b := builder{levels: levels, limits: l, tables: tables, inTable: -1}
	for _, line := range lines {
		b.take(line)
	}
	b.flush()
	return b.out
}

// a builder accumulates the blocks of one page.
type builder struct {
	out    []richdoc.Block
	levels map[int]int
	limits Limits
	tables []extract.Table

	para        []string      // the lines of the paragraph being built
	items       []string      // the text of each item of the list being built
	mark        string        // the mark that list's items begin with
	prev        []extract.Run // the line before this one
	inTable     int           // the index of the table last emitted, or -1
	headingSize int           // the size of the heading last put out
}

// take reads one line.
func (b *builder) take(line []extract.Run) {
	defer func() { b.prev = line }()

	said := extract.Join(line)
	if strings.TrimSpace(said) == "" {
		return
	}
	// ⛔ The heading is decided BEFORE the table, and the two agree because
	// the tables were found among the lines that are not headings — see
	// [tablesBelowTheHeadings]. Asking the other way round let a heading
	// between two rows of a table become a row of it.
	if level, ok := b.levels[round(lineSize(line))]; ok {
		// A heading's text wraps like anything else, and the lines it wraps
		// onto are the same heading.
		//
		// ⛔ The guard is the same SIZE and not the same level. Levels are
		// clamped to as many as the format has, so three headings at thirty,
		// twenty-eight and twenty-six points can share a level — and joining
		// those made one heading out of three. The distance is the other
		// guard: a wrapped heading's next line is one leading below, and a
		// second heading sits further down.
		size := round(lineSize(line))
		if h, ok := b.lastHeading(); ok && b.headingSize == size && !b.far(line) {
			b.out[len(b.out)-1] = richdoc.Heading{
				Level: h.Level, Inlines: text(join(inlineText(h.Inlines), said)),
			}
			return
		}
		b.flush()
		b.out = append(b.out, richdoc.Heading{Level: level, Inlines: text(said)})
		b.headingSize = size
		return
	}
	if i, ok := b.tableAt(line); ok {
		// A table's lines all fall inside its box, so it is emitted on the
		// first of them and skipped on every one after.
		if i != b.inTable {
			b.flush()
			b.out = append(b.out, asTable(b.tables[i]))
			b.inTable = i
		}
		return
	}
	if mark, rest, ok := listMark(line); ok {
		if len(b.items) > 0 && mark != b.mark {
			b.flush()
		}
		b.mark = mark
		b.items = append(b.items, rest)
		return
	}
	if b.far(line) {
		b.flush()
	}
	if len(b.items) > 0 {
		// An unmarked line under a list item is the rest of that item: a
		// bullet's text wraps, and the lines it wraps onto carry no bullet.
		b.items[len(b.items)-1] = join(b.items[len(b.items)-1], said)
		return
	}
	b.para = append(b.para, said)
}

// lastHeading is the heading this page ended with, if the last thing it put
// out was one.
func (b *builder) lastHeading() (richdoc.Heading, bool) {
	if len(b.out) == 0 || len(b.para) > 0 || len(b.items) > 0 {
		return richdoc.Heading{}, false
	}
	h, ok := b.out[len(b.out)-1].(richdoc.Heading)
	return h, ok
}

// inlineText reads back inline content this package wrote.
func inlineText(in []richdoc.Inline) string {
	var out strings.Builder
	for _, i := range in {
		if t, ok := i.(richdoc.Text); ok {
			out.WriteString(t.Value)
		}
	}
	return out.String()
}

// joinAll puts every line of a block together, each the way [join] does.
func joinAll(lines []string) string {
	out := lines[0]
	for _, line := range lines[1:] {
		out = join(out, line)
	}
	return out
}

// join puts two lines of one block together.
//
// ⛔ No space after a hyphen, because that is where a typesetter broke a word:
// "cadre man-" and "ager" joined with a space is "man- ager", which is not
// what the page says. Removing the line break says "man-ager", which is.
//
// ⛔ And the hyphen STAYS. Dropping it would read "manager", which is right
// here and wrong for "well-known" broken at its own hyphen — and the two
// cannot be told apart without a dictionary of the language the document is
// in. Keeping it loses nothing a reader cannot undo; dropping it cannot be
// undone.
func join(before, next string) string {
	if strings.HasSuffix(before, "-") {
		return before + next
	}
	return before + " " + next
}

// flush closes whatever block is open.
func (b *builder) flush() {
	if len(b.para) > 0 {
		b.out = append(b.out, richdoc.Paragraph{Inlines: text(joinAll(b.para))})
		b.para = nil
	}
	if len(b.items) > 0 {
		items := make([]richdoc.ListItem, len(b.items))
		for i, s := range b.items {
			items[i] = richdoc.ListItem{
				Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: text(s)}},
			}
		}
		b.out = append(b.out, richdoc.List{Tight: true, Items: items})
		b.items, b.mark = nil, ""
	}
}

// tableAt is the index of the table a line belongs to, if any. A table's box
// is where its words are, so a line of it sits inside that box.
//
// ⛔ The test is vertical only. Two tables side by side on one page would each
// claim the other's lines, and this reads them as one — which is the honest
// limit of a box that says where the words are rather than which words.
func (b *builder) tableAt(line []extract.Run) (int, bool) {
	y := line[0].Y
	for i, t := range b.tables {
		if y >= t.Y && y <= t.Y+t.Height {
			return i, true
		}
	}
	return 0, false
}

// far reports that a line sits far enough below the one before it to start
// something new. The first line of a page has nothing above it, so it is never
// far from it.
func (b *builder) far(line []extract.Run) bool {
	if len(b.prev) == 0 {
		return false
	}
	size := maxSize(b.prev)
	if size <= 0 {
		// Text set at no size says nothing about how far apart its lines
		// should be, so this has no opinion either.
		return false
	}
	return b.prev[0].Y-line[0].Y > b.limits.ParagraphBreak*size
}

// asTable turns a found grid into a richdoc table, first row as the header.
//
// ⛔ The first row becomes the header because every format richdoc writes to
// puts one there, and a grid found by looking at alignment does not know
// whether its first row is one. This picks the reading that renders.
func asTable(t extract.Table) richdoc.Table {
	out := richdoc.Table{Header: cells(t.Rows[0])}
	for _, row := range t.Rows[1:] {
		out.Rows = append(out.Rows, cells(row))
	}
	return out
}

func cells(row []string) []richdoc.Cell {
	out := make([]richdoc.Cell, len(row))
	for i, s := range row {
		// Inlines alone, Blocks left nil: one line of text is one paragraph,
		// which is the case richdoc.Cell says to do this for.
		out[i] = richdoc.Cell{Inlines: text(s)}
	}
	return out
}

// text is one string as inline content. An empty string is no content rather
// than an empty Text, so a cell nothing was written in is an empty cell.
func text(s string) []richdoc.Inline {
	if s == "" {
		return nil
	}
	return []richdoc.Inline{richdoc.Text{Value: s}}
}

// lineSize is the size most of a line's CHARACTERS are set at, which is what
// decides whether the line is a heading.
//
// ⛔ Not the tallest piece on it. A heading with a small footnote mark in it is
// still a heading either way — the heading is most of the characters — but a
// paragraph with one large glyph in it is a PARAGRAPH, and reading the tallest
// piece makes it a heading. It is the same measure the body size is found
// with, which is the point: the line and the document are weighed alike.
func lineSize(line []extract.Run) float64 {
	chars := map[float64]int{}
	for _, r := range line {
		chars[r.Size] += len([]rune(r.Text))
	}
	out, most := line[0].Size, 0
	for size, n := range chars {
		// A tie goes to the LARGER size, so that a two-word heading beside a
		// two-word label does not lose by the order of a map walk.
		if n > most || (n == most && size > out) {
			out, most = size, n
		}
	}
	return out
}

// maxSize is the tallest piece of text on a line, which is what says how far
// apart two lines should be: leading is set from the tallest thing on the
// line, not from most of it.
func maxSize(line []extract.Run) float64 {
	out := line[0].Size
	for _, r := range line {
		if r.Size > out {
			out = r.Size
		}
	}
	return out
}

// listMark is the mark a line begins with and the rest of the line, if the
// line begins with one.
//
// A mark is short and has no letter or digit in it. ⛔ That excludes "1." and
// "a)", the marks of a NUMBERED list, and the exclusion is deliberate: taking
// them would mean deciding that "1990" at the start of a line is a list
// marker, and a table of dates is nothing but those. A numbered list read this
// way keeps its numbers inside its items, which is what every Markdown
// renderer would have renumbered anyway.
func listMark(line []extract.Run) (mark, rest string, ok bool) {
	if len(line) < 2 {
		return "", "", false
	}
	first := strings.TrimSpace(line[0].Text)
	if first == "" || len([]rune(first)) > markRunes {
		return "", "", false
	}
	for _, r := range first {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return "", "", false
		}
	}
	return first, extract.Join(line[1:]), true
}
