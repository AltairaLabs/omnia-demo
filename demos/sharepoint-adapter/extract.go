package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Section mirrors one entry of the batch contract's documents[].sections[]
// (memory-api's IngestSection): a heading, its depth (1 = top) and the text
// under it. memory-api chunks within a section and never across one, and
// writes a section entity per section, so structure the extractor can see
// is structure the agent can cite.
type Section struct {
	Heading string `json:"heading,omitempty"`
	Level   int    `json:"level,omitempty"`
	Text    string `json:"text"`
}

// Extracted is a document as the adapter hands it on: the flat text (what the
// agent's fetch tool reads) and, for formats that carry structure, the
// sections memory-api should split by. Markdown and plain text carry no
// sections: memory-api splits Markdown on its headings itself.
type Extracted struct {
	Text     string
	Sections []Section
}

// extractDocument converts raw document bytes into text and sections.
//
//	.docx — a section per Heading-styled paragraph (Title counts as level 1)
//	.pptx — a section per slide, the slide's first paragraph as its heading
//	.xlsx — a section per sheet, the sheet as a Markdown table
//
// Every other extension (.txt, .md, unknown) is returned as-is.
func extractDocument(filename string, raw []byte) (Extracted, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	var (
		sections []Section
		err      error
	)
	switch ext {
	case ".docx":
		sections, err = extractDocx(raw)
	case ".pptx":
		sections, err = extractPptx(raw)
	case ".xlsx":
		sections, err = extractXlsx(raw)
	default:
		return Extracted{Text: string(raw)}, nil
	}
	if err != nil {
		return Extracted{}, err
	}
	return Extracted{Text: joinSections(sections), Sections: sections}, nil
}

// extractText is the flat-text view, for callers that only want prose.
func extractText(filename string, raw []byte) (string, error) {
	ex, err := extractDocument(filename, raw)
	return ex.Text, err
}

// joinSections renders sections as readable text: heading, blank line, body.
func joinSections(sections []Section) string {
	var b strings.Builder
	for _, s := range sections {
		if s.Heading != "" {
			b.WriteString(s.Heading)
			b.WriteString("\n\n")
		}
		if s.Text != "" {
			b.WriteString(s.Text)
			b.WriteString("\n\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// docxDocumentPart is the OOXML part holding a Word document's body text.
const docxDocumentPart = "word/document.xml"

// headingStyle matches Word's built-in heading style ids ("Heading1",
// "heading 2", "Title") and yields the level; Title is level 1.
var headingStyle = regexp.MustCompile(`(?i)^(heading|title)\s*(\d*)$`)

func headingLevel(styleID string) (int, bool) {
	m := headingStyle.FindStringSubmatch(strings.TrimSpace(styleID))
	if m == nil {
		return 0, false
	}
	if m[2] == "" {
		return 1, true
	}
	n, _ := strconv.Atoi(m[2])
	if n < 1 {
		n = 1
	}
	if n > 6 {
		n = 6
	}
	return n, true
}

// extractDocx reads word/document.xml into sections: a paragraph carrying a
// Heading style starts a section; every other paragraph is body text of the
// current one. Text before the first heading is an unnamed section.
func extractDocx(raw []byte) ([]Section, error) {
	parts, err := zipParts(raw, func(name string) bool { return name == docxDocumentPart })
	if err != nil {
		return nil, fmt.Errorf("docx extraction: %w", err)
	}
	var out []Section
	for _, part := range parts {
		sections, err := docxSections(bytes.NewReader(part))
		if err != nil {
			return nil, fmt.Errorf("docx extraction: %w", err)
		}
		out = append(out, sections...)
	}
	return out, nil
}

// docxWalker accumulates paragraphs into sections while streaming the XML.
type docxWalker struct {
	sections []Section
	cur      Section
	body     []string
	// paragraph state
	inPara    bool
	paraText  strings.Builder
	paraStyle string
	capture   bool
}

func (w *docxWalker) start(el xml.StartElement) {
	switch el.Name.Local {
	case "p":
		w.inPara, w.paraStyle = true, ""
		w.paraText.Reset()
	case "pStyle":
		if v, ok := attr(el, "val"); ok {
			w.paraStyle = v
		}
	case "t":
		w.capture = true
	case "tab":
		if w.inPara {
			w.paraText.WriteByte('\t')
		}
	case "br":
		if w.inPara {
			w.paraText.WriteByte('\n')
		}
	}
}

func (w *docxWalker) end(local string) {
	switch local {
	case "t":
		w.capture = false
	case "p":
		w.endParagraph()
	}
}

func (w *docxWalker) endParagraph() {
	w.inPara = false
	text := strings.TrimSpace(w.paraText.String())
	if text == "" {
		return
	}
	if level, ok := headingLevel(w.paraStyle); ok {
		w.flush()
		w.cur = Section{Heading: text, Level: level}
		return
	}
	w.body = append(w.body, text)
}

func (w *docxWalker) flush() {
	w.cur.Text = strings.Join(w.body, "\n")
	if w.cur.Text != "" || w.cur.Heading != "" {
		w.sections = append(w.sections, w.cur)
	}
	w.cur, w.body = Section{}, nil
}

func (w *docxWalker) char(data []byte) {
	if w.capture && w.inPara {
		w.paraText.Write(data)
	}
}

func docxSections(r io.Reader) ([]Section, error) {
	var w docxWalker
	if err := walkXML(r, &w); err != nil {
		return nil, err
	}
	w.flush()
	return w.sections, nil
}

// slidePart matches ppt/slides/slideN.xml and captures N for ordering: the
// zip lists slide10 before slide2.
var slidePart = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

// extractPptx makes one section per slide: the slide's first paragraph is
// its heading (the title placeholder comes first in the shape tree), the
// rest its body.
func extractPptx(raw []byte) ([]Section, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("pptx extraction: %w", err)
	}
	type slide struct {
		n    int
		file *zip.File
	}
	var slides []slide
	for _, f := range zr.File {
		if m := slidePart.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			slides = append(slides, slide{n, f})
		}
	}
	sort.Slice(slides, func(i, j int) bool { return slides[i].n < slides[j].n })
	var out []Section
	for _, s := range slides {
		paras, err := readZipFile(s.file, func(r io.Reader) ([]string, error) { return extractParagraphText(r, "p", "t") })
		if err != nil {
			return nil, fmt.Errorf("pptx extraction: %w", err)
		}
		lines := nonEmptyLines(strings.Join(paras, ""))
		if len(lines) == 0 {
			continue
		}
		out = append(out, Section{Heading: lines[0], Level: 1, Text: strings.Join(lines[1:], "\n")})
	}
	return out, nil
}

func nonEmptyLines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// worksheetPart matches xl/worksheets/sheetN.xml.
var worksheetPart = regexp.MustCompile(`^xl/worksheets/sheet(\d+)\.xml$`)

// extractXlsx makes one section per worksheet, rendered as a Markdown pipe
// table (first row = header) so memory-api chunks it by rows with the
// header repeated. Sheet names come from xl/workbook.xml. A workbook that
// carries only a shared-string table (no worksheets) degrades to those
// strings as one unnamed section.
func extractXlsx(raw []byte) ([]Section, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("xlsx extraction: %w", err)
	}
	files := map[string]*zip.File{}
	var sheets []int
	for _, f := range zr.File {
		files[f.Name] = f
		if m := worksheetPart.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			sheets = append(sheets, n)
		}
	}
	shared, err := xlsxSharedStrings(files["xl/sharedStrings.xml"])
	if err != nil {
		return nil, fmt.Errorf("xlsx extraction: %w", err)
	}
	if len(sheets) == 0 {
		if len(shared) == 0 {
			return nil, nil
		}
		return []Section{{Text: strings.Join(shared, " ")}}, nil
	}
	sort.Ints(sheets)
	names, _ := xlsxSheetNames(files["xl/workbook.xml"])
	var out []Section
	for i, n := range sheets {
		rows, err := readZipFile(files[fmt.Sprintf("xl/worksheets/sheet%d.xml", n)],
			func(r io.Reader) ([][]string, error) { return xlsxRows(r, shared) })
		if err != nil {
			return nil, fmt.Errorf("xlsx extraction: %w", err)
		}
		if len(rows) == 0 {
			continue
		}
		name := fmt.Sprintf("Sheet %d", n)
		if i < len(names) {
			name = names[i]
		}
		out = append(out, Section{Heading: name, Level: 1, Text: markdownTable(rows)})
	}
	return out, nil
}

// xlsxSharedStrings reads the shared-string table; nil file → none.
func xlsxSharedStrings(f *zip.File) ([]string, error) {
	if f == nil {
		return nil, nil
	}
	return readZipFile(f, func(r io.Reader) ([]string, error) { return xlsxSharedItems(r) })
}

// xmlHandler receives the tokens walkXML streams.
type xmlHandler interface {
	start(el xml.StartElement)
	char(data []byte)
	end(local string)
}

// walkXML streams r's tokens into h.
func walkXML(r io.Reader, h xmlHandler) error {
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch v := tok.(type) {
		case xml.StartElement:
			h.start(v)
		case xml.CharData:
			h.char(v)
		case xml.EndElement:
			h.end(v.Name.Local)
		}
	}
}

// sharedStringsWalker joins the <t> runs of each <si> (rich-text cells carry
// several) into one string per item.
type sharedStringsWalker struct {
	items   []string
	cur     strings.Builder
	inItem  bool
	capture bool
}

func (w *sharedStringsWalker) start(el xml.StartElement) {
	switch el.Name.Local {
	case "si":
		w.inItem = true
		w.cur.Reset()
	case "t":
		w.capture = w.inItem
	}
}

func (w *sharedStringsWalker) char(data []byte) {
	if w.capture {
		w.cur.Write(data)
	}
}

func (w *sharedStringsWalker) end(local string) {
	switch local {
	case "t":
		w.capture = false
	case "si":
		w.items = append(w.items, w.cur.String())
		w.inItem = false
	}
}

func xlsxSharedItems(r io.Reader) ([]string, error) {
	var w sharedStringsWalker
	if err := walkXML(r, &w); err != nil {
		return nil, err
	}
	return w.items, nil
}

// sheetNamesWalker collects <sheet name="…"> in order.
type sheetNamesWalker struct{ names []string }

func (w *sheetNamesWalker) start(el xml.StartElement) {
	if el.Name.Local != "sheet" {
		return
	}
	if v, ok := attr(el, "name"); ok {
		w.names = append(w.names, v)
	}
}
func (w *sheetNamesWalker) char([]byte) {}
func (w *sheetNamesWalker) end(string)  {}

// attr returns the named attribute of el, namespace ignored.
func attr(el xml.StartElement, name string) (string, bool) {
	for _, a := range el.Attr {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

// xlsxSheetNames reads the <sheet name="…"> entries of xl/workbook.xml in
// order; nil file → none.
func xlsxSheetNames(f *zip.File) ([]string, error) {
	if f == nil {
		return nil, nil
	}
	return readZipFile(f, func(r io.Reader) ([]string, error) {
		var w sheetNamesWalker
		if err := walkXML(r, &w); err != nil {
			return nil, err
		}
		return w.names, nil
	})
}

// rowsWalker reads a worksheet's <row>/<c> cells in order, resolving shared
// (t="s") and inline (t="inlineStr") strings; other cells keep their <v>.
type rowsWalker struct {
	shared   []string
	rows     [][]string
	row      []string
	cellType string
	val      strings.Builder
	capture  bool
	inRow    bool
}

func (w *rowsWalker) start(el xml.StartElement) {
	switch el.Name.Local {
	case "row":
		w.inRow, w.row = true, nil
	case "c":
		w.cellType, _ = attr(el, "t")
		w.val.Reset()
	case "v", "t":
		w.capture = w.inRow
	}
}

func (w *rowsWalker) char(data []byte) {
	if w.capture {
		w.val.Write(data)
	}
}

func (w *rowsWalker) end(local string) {
	switch local {
	case "v", "t":
		w.capture = false
	case "c":
		w.row = append(w.row, cellValue(w.cellType, w.val.String(), w.shared))
	case "row":
		if strings.Join(w.row, "") != "" {
			w.rows = append(w.rows, w.row)
		}
		w.inRow = false
	}
}

func xlsxRows(r io.Reader, shared []string) ([][]string, error) {
	w := rowsWalker{shared: shared}
	if err := walkXML(r, &w); err != nil {
		return nil, err
	}
	return w.rows, nil
}

func cellValue(cellType, raw string, shared []string) string {
	if cellType == "s" {
		if idx, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && idx >= 0 && idx < len(shared) {
			return shared[idx]
		}
		return ""
	}
	return strings.TrimSpace(raw)
}

// markdownTable renders rows as a pipe table with the first row as header,
// padding short rows to the header's width. Pipes in cells are escaped.
func markdownTable(rows [][]string) string {
	width := len(rows[0])
	cell := func(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "|", `\|`) }
	line := func(r []string) string {
		cells := make([]string, width)
		for i := range cells {
			if i < len(r) {
				cells[i] = cell(r[i])
			}
		}
		return "| " + strings.Join(cells, " | ") + " |"
	}
	lines := make([]string, 0, len(rows)+1)
	lines = append(lines, line(rows[0]), "|"+strings.Repeat("---|", width))
	for _, r := range rows[1:] {
		lines = append(lines, line(r))
	}
	return strings.Join(lines, "\n")
}

// zipParts returns the raw bytes of every zip entry matching the predicate.
func zipParts(raw []byte, match func(string) bool) ([][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, err
	}
	var out [][]byte
	for _, f := range zr.File {
		if !match(f.Name) {
			continue
		}
		b, err := readZipFile(f, io.ReadAll)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// readZipFile opens one zip entry and applies fn to its contents.
func readZipFile[T any](f *zip.File, fn func(io.Reader) (T, error)) (T, error) {
	var zero T
	if f == nil {
		return zero, fmt.Errorf("zip entry missing")
	}
	rc, err := f.Open()
	if err != nil {
		return zero, err
	}
	defer func() { _ = rc.Close() }()
	return fn(rc)
}

// paraState tracks running state while walking paragraph XML.
type paraState struct {
	tokens      []string
	capture     bool
	inPara      bool
	paraHasText bool
}

func (s *paraState) onStart(local, paraLocal, textLocal string) {
	switch local {
	case paraLocal:
		s.inPara = true
		s.paraHasText = false
	case textLocal:
		s.capture = true
	}
}

func (s *paraState) onChar(data string) {
	if s.capture {
		s.tokens = append(s.tokens, data)
		if s.inPara {
			s.paraHasText = true
		}
	}
}

func (s *paraState) onEnd(local, paraLocal, textLocal string) {
	switch local {
	case textLocal:
		s.capture = false
	case paraLocal:
		if s.inPara && s.paraHasText {
			s.tokens = append(s.tokens, "\n")
		}
		s.inPara = false
		s.paraHasText = false
	}
}

// extractParagraphText streams XML from r, collecting text from <textLocal>
// elements and appending a newline sentinel at each </paraLocal> close tag.
// The caller joins the returned slice with "" to get paragraph-separated text.
func extractParagraphText(r io.Reader, paraLocal, textLocal string) ([]string, error) {
	dec := xml.NewDecoder(r)
	var s paraState
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch v := tok.(type) {
		case xml.StartElement:
			s.onStart(v.Name.Local, paraLocal, textLocal)
		case xml.CharData:
			s.onChar(string(v))
		case xml.EndElement:
			s.onEnd(v.Name.Local, paraLocal, textLocal)
		}
	}
	// Trim trailing newline token if present
	if len(s.tokens) > 0 && s.tokens[len(s.tokens)-1] == "\n" {
		s.tokens = s.tokens[:len(s.tokens)-1]
	}
	return s.tokens, nil
}
