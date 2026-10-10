package main

// doc-Plugin, Teil 3: Konvertieren zwischen .docx und .odt.
//
//   doc.Convert(quelle, ziel) -> bool
//
// Die Quelle wird in das interne Dokumentmodell (docState, siehe Schreiben)
// eingelesen und mit demselben Code in das Zielformat geschrieben.
//
// Das Modell kennt: Schriftart/-größe, Überschriften 1-3, Absätze (Fett/Kursiv/
// Unterstrichen für den ganzen Absatz), Aufzählungen, nummerierte Listen,
// Tabellen (mit Kopfzeile) und Seitenumbrüche. Für Dokumente, die dieses Plugin
// erzeugt hat, ist die Konvertierung deshalb verlustfrei. Fremde Dokumente
// werden nach bestem Wissen übernommen, alles andere (Bilder, Fußnoten, Kopf-/
// Fußzeilen, gemischte Formatierung innerhalb eines Absatzes ...) geht verloren.

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
)

// ------------------------------------------------------------
// Handler
// ------------------------------------------------------------

func handleConvert(args []jsonValue) []byte {
	const fn = "doc.Convert"

	src, err := getStringArg(args, 0, fn, true)
	if err != nil {
		return errorResult(err.Error())
	}

	dst, err := getStringArg(args, 1, fn, true)
	if err != nil {
		return errorResult(err.Error())
	}

	srcExt := strings.ToLower(filepath.Ext(src))
	dstExt := strings.ToLower(filepath.Ext(dst))

	if (srcExt != ".docx" && srcExt != ".odt") || (dstExt != ".docx" && dstExt != ".odt") {
		return errorResult(fn + ": nur .docx und .odt werden unterstützt")
	}

	if srcExt == dstExt {
		return errorResult(fn + ": Quelle und Ziel haben dasselbe Format")
	}

	d, err := readDocState(src)
	if err != nil {
		return errorResult(fmt.Sprintf("%s: %v", fn, err))
	}

	if err := saveDoc(d, dst); err != nil {
		return errorResult(fmt.Sprintf("%s: %v", fn, err))
	}

	return boolResult(true)
}

// ------------------------------------------------------------
// Dokument in das interne Modell einlesen
// ------------------------------------------------------------

func openPart(zr *zip.ReadCloser, name string) (io.ReadCloser, error) {
	for _, f := range zr.File {
		if f.Name == name {
			return f.Open()
		}
	}

	return nil, fmt.Errorf("%s nicht im Archiv gefunden (keine gültige Datei?)", name)
}

func readDocState(path string) (*docState, error) {
	odt := strings.ToLower(filepath.Ext(path)) == ".odt"

	bodyName, stylesName := "word/document.xml", "word/styles.xml"
	if odt {
		bodyName, stylesName = "content.xml", "styles.xml"
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("pfad '%s' konnte nicht aufgelöst werden: %w", path, err)
	}

	zr, err := zip.OpenReader(abs)
	if err != nil {
		return nil, fmt.Errorf("datei konnte nicht geöffnet werden: %w", err)
	}
	defer zr.Close()

	d := newDocState()

	// Schrift und Größe sind optional: fehlt die Styles-Datei, bleiben die Standardwerte.
	if rc, err := openPart(zr, stylesName); err == nil {
		font, size := parseDocFont(io.LimitReader(rc, maxXMLBytes), odt)
		rc.Close()

		font = strings.Join(strings.Fields(fontClean.Replace(cleanText(font))), " ")
		if font != "" && len(font) <= 100 {
			d.font = font
		}

		if size >= 4 && size <= 200 {
			d.size = size
		}
	}

	rc, err := openPart(zr, bodyName)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	blocks, err := parseDocBlocks(io.LimitReader(rc, maxXMLBytes), odt)
	if err != nil {
		return nil, err
	}

	d.blocks = blocks

	return d, nil
}

func parsePt(s string) float64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "pt")

	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}

	return v
}

// parseDocFont liest Standard-Schriftart und -größe.
// docx: docDefaults/rPrDefault (rFonts ascii, sz in Halbpunkten)
// odt:  default-style (paragraph) / text-properties (font-name, font-size)
func parseDocFont(r io.Reader, odt bool) (string, float64) {
	dec := xml.NewDecoder(r)

	var (
		font      string
		size      float64
		inDefault bool
	)

	for {
		tok, err := dec.Token()
		if err != nil {
			break // EOF oder Fehler: nimm, was bis hierhin gefunden wurde
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "rPrDefault":
				inDefault = !odt

			case "default-style":
				if odt && attr(t, "family") == "paragraph" {
					inDefault = true
				}

			case "rFonts":
				if inDefault && !odt && font == "" {
					font = attr(t, "ascii")
				}

			case "sz":
				if inDefault && !odt && size == 0 {
					if v, err := strconv.ParseFloat(attr(t, "val"), 64); err == nil {
						size = v / 2
					}
				}

			case "text-properties":
				if inDefault && odt {
					if font == "" {
						font = attr(t, "font-name")
					}
					if size == 0 {
						size = parsePt(attr(t, "font-size"))
					}
				}
			}

		case xml.EndElement:
			if t.Name.Local == "rPrDefault" || t.Name.Local == "default-style" {
				inDefault = false
			}
		}
	}

	return font, size
}

// headingLevel erkennt Überschriften-Formate ("Heading1", "berschrift2" ...).
func headingLevel(style string) int {
	s := strings.ToLower(style)

	if !strings.Contains(s, "heading") && !strings.Contains(s, "berschrift") {
		return 0
	}

	if c := s[len(s)-1]; c >= '1' && c <= '9' {
		return int(c - '0')
	}

	return 1
}

// decodeSpan liest die Formatierung aus den Span-Namen dieses Plugins
// (Tb, Ti, Tu, Tbi, Tbu, Tiu, Tbiu).
func decodeSpan(name string) (b, i, u bool) {
	if len(name) < 2 || name[0] != 'T' {
		return false, false, false
	}

	for _, c := range name[1:] {
		switch c {
		case 'b':
			b = true
		case 'i':
			i = true
		case 'u':
			u = true
		default:
			return false, false, false
		}
	}

	return b, i, u
}

func padRows(rows [][]string) [][]string {
	width := 0

	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}

	for i := range rows {
		for len(rows[i]) < width {
			rows[i] = append(rows[i], "")
		}
	}

	return rows
}

// parseDocBlocks liest docx (w:p, w:tbl ...) und odt (text:p, text:h, table:table ...)
// in eine Liste von docBlock.
func parseDocBlocks(r io.Reader, odt bool) ([]docBlock, error) {
	dec := xml.NewDecoder(r)

	var (
		blocks []docBlock
		stack  []string

		// aktueller Absatz
		paraDepth                          int
		text                               strings.Builder
		pStyle                             string
		numID                              string
		hLevel                             int
		pageBreak                          bool
		tokens, nBold, nItalic, nUnderline int

		// Formatierung des aktuellen Runs (docx) bzw. Spans (odt)
		runB, runI, runU bool
		inT              bool

		// Listen
		listDepth int
		curList   *docBlock // odt
		lastNum   string    // docx: numId der zuletzt gelesenen Liste

		// Tabellen
		tblDepth  int
		rows      [][]string
		row       []string
		cell      []string
		tblHeader bool

		afterTable bool
	)

	resetPara := func() {
		text.Reset()
		pStyle = ""
		numID = ""
		hLevel = 0
		pageBreak = false
		tokens, nBold, nItalic, nUnderline = 0, 0, 0, 0
	}

	finishPara := func() {
		txt := text.String()

		switch {
		case tblDepth > 0:
			cell = append(cell, txt)

		case odt && listDepth > 0:
			if curList != nil {
				curList.items = append(curList.items, txt)
			}

		case !odt && numID != "" && numID != "0" && headingLevel(pStyle) == 0:
			// docx-Listenpunkt: aufeinanderfolgende Punkte mit gleicher numId = eine Liste.
			// numId 1 ist die Aufzählung, jede weitere eine eigene nummerierte Liste.
			if len(blocks) > 0 && lastNum == numID && blocks[len(blocks)-1].kind == "list" {
				last := &blocks[len(blocks)-1]
				last.items = append(last.items, txt)
			} else {
				blocks = append(blocks, docBlock{
					kind:     "list",
					items:    []string{txt},
					numbered: numID != "1",
				})
				lastNum = numID
			}

			afterTable = false

		default:
			lastNum = ""

			level := hLevel
			if !odt {
				level = headingLevel(pStyle)
			}

			switch {
			case level > 0:
				if level > 3 {
					level = 3
				}

				blocks = append(blocks, docBlock{kind: "h", level: level, text: txt})
				afterTable = false

			case pageBreak && strings.TrimSpace(txt) == "":
				blocks = append(blocks, docBlock{kind: "pb"})
				afterTable = false

			case txt == "" && afterTable:
				// Leerer Absatz direkt nach einer Tabelle: den fügt der Schreiber
				// selbst wieder ein, sonst würde er sich bei jeder Konvertierung verdoppeln.
				afterTable = false

			default:
				blocks = append(blocks, docBlock{
					kind:      "p",
					text:      txt,
					bold:      tokens > 0 && nBold == tokens,
					italic:    tokens > 0 && nItalic == tokens,
					underline: tokens > 0 && nUnderline == tokens,
				})
				afterTable = false
			}
		}

		resetPara()
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {

		case xml.StartElement:
			n := t.Name.Local

			// odt: Kommentare und Fußnoten nicht in den Fließtext mischen
			if odt && (n == "annotation" || n == "note") {
				if err := dec.Skip(); err != nil {
					return nil, err
				}
				continue
			}

			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			stack = append(stack, n)

			switch n {
			case "p", "h":
				paraDepth++

				if paraDepth == 1 {
					resetPara()

					if odt {
						pStyle = attr(t, "style-name")

						if n == "h" {
							hLevel, _ = strconv.Atoi(attr(t, "outline-level"))
							if hLevel < 1 {
								hLevel = 1
							}
						}

						if pStyle == "PB" {
							pageBreak = true
						}
					}
				}

			case "pStyle":
				if !odt && paraDepth > 0 && pStyle == "" {
					pStyle = attr(t, "val")
				}

			case "numId":
				if !odt && paraDepth > 0 && parent == "numPr" {
					numID = attr(t, "val")
				}

			case "r":
				if !odt {
					runB, runI, runU = false, false, false
				}

			case "b", "i", "u":
				if !odt && paraDepth > 0 && parent == "rPr" {
					v := attr(t, "val")
					on := v != "0" && v != "false" && v != "none"

					switch n {
					case "b":
						runB = on
					case "i":
						runI = on
					case "u":
						runU = on
					}
				}

			case "span":
				if odt {
					runB, runI, runU = decodeSpan(attr(t, "style-name"))
				}

			case "t":
				if !odt {
					inT = true
				}

			case "tab":
				if paraDepth > 0 && parent != "tabs" {
					text.WriteByte('\t')
				}

			case "br", "line-break":
				if paraDepth > 0 {
					if attr(t, "type") == "page" {
						pageBreak = true
					} else {
						text.WriteByte('\n')
					}
				}

			case "s":
				if odt && paraDepth > 0 {
					c := 1

					if v := attr(t, "c"); v != "" {
						if x, err := strconv.Atoi(v); err == nil {
							c = x
						}
					}

					if c < 0 {
						c = 0
					}
					if c > 10000 {
						c = 10000
					}

					text.WriteString(strings.Repeat(" ", c))
				}

			case "list":
				if odt {
					listDepth++

					if listDepth == 1 && tblDepth == 0 {
						curList = &docBlock{
							kind:     "list",
							numbered: attr(t, "style-name") == "LNum",
						}
					}
				}

			case "tbl", "table":
				tblDepth++

				if tblDepth == 1 {
					rows = [][]string{}
					tblHeader = false
				}

			case "table-header-rows":
				if odt && tblDepth == 1 {
					tblHeader = true
				}

			case "tblHeader":
				if !odt && tblDepth == 1 && len(rows) == 0 {
					tblHeader = true
				}

			case "tr", "table-row":
				if tblDepth == 1 {
					row = nil
				}

			case "tc", "table-cell", "covered-table-cell":
				if tblDepth == 1 {
					cell = nil
				}
			}

		case xml.EndElement:
			n := t.Name.Local

			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

			switch n {
			case "t":
				inT = false

			case "r", "span":
				runB, runI, runU = false, false, false

			case "p", "h":
				if paraDepth > 0 {
					paraDepth--

					if paraDepth == 0 {
						finishPara()
					}
				}

			case "list":
				if odt && listDepth > 0 {
					listDepth--

					if listDepth == 0 && curList != nil {
						if len(curList.items) > 0 {
							blocks = append(blocks, *curList)
							lastNum = ""
							afterTable = false
						}

						curList = nil
					}
				}

			case "tc", "table-cell", "covered-table-cell":
				if tblDepth == 1 {
					row = append(row, strings.Join(cell, "\n"))
				}

			case "tr", "table-row":
				if tblDepth == 1 {
					rows = append(rows, row)
				}

			case "tbl", "table":
				if tblDepth > 0 {
					if tblDepth == 1 {
						rows = padRows(rows)

						if len(rows) > 0 && len(rows[0]) > 0 {
							blocks = append(blocks, docBlock{
								kind:   "table",
								rows:   rows,
								header: tblHeader,
							})

							lastNum = ""
							afterTable = true
						}
					}

					tblDepth--
				}
			}

		case xml.CharData:
			if paraDepth > 0 && (odt || inT) && len(t) > 0 {
				text.Write(t)

				tokens++

				if runB {
					nBold++
				}
				if runI {
					nItalic++
				}
				if runU {
					nUnderline++
				}
			}
		}
	}

	return blocks, nil
}
