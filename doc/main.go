package main

// doc-Plugin: Lesen und Schreiben von .docx und .odt.
//
// Lesen:
//   doc.ReadText(pfad [, tabellenMarker]) -> string
//   doc.Tables(pfad) -> array
//
// Schreiben:
//   doc.New() -> bool
//   doc.Font(name [, size]) -> bool
//   doc.Heading(text, level) -> bool
//   doc.Text(text [, bold] [, italic] [, underline]) -> bool
//   doc.List(array [, numbered]) -> bool
//   doc.Table(array [, header]) -> bool
//   doc.PageBreak() -> bool
//   doc.Save(pfad) -> bool
//   doc.Write(pfad, text) -> bool
//
// Das Dokument wird beim Schreiben im Speicher aufgebaut.
// Das Format ergibt sich aus der Dateiendung.
//
// Nur Go-Stdlib, keine externen Abhängigkeiten.
// Bauen wie bei den anderen Plugins:
//   GOOS=wasip1 GOARCH=wasm go build -o doc.wasm .

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/tsawler/tabula"
)

const pluginNamespace = "doc"

const maxXMLBytes = 256 << 20

// ------------------------------------------------------------
// WASM-ABI / Speicher
// ------------------------------------------------------------

var liveBuffers = map[uint32][]byte{}

func dosDateTime(t time.Time) (date, tm uint16) {
	date = uint16(t.Year()-1980)<<9 | uint16(t.Month())<<5 | uint16(t.Day())
	tm = uint16(t.Hour())<<11 | uint16(t.Minute())<<5 | uint16(t.Second()/2)
	return date, tm
}

func alloc(size uint32) uint32 {
	buf := make([]byte, size)
	if size == 0 {
		buf = make([]byte, 1)
	}

	ptr := uint32(uintptr(unsafe.Pointer(&buf[0])))
	liveBuffers[ptr] = buf

	return ptr
}

//go:wasmexport alloc
func exportAlloc(size uint32) uint32 {
	return alloc(size)
}

//go:wasmexport dealloc
func dealloc(ptr uint32, size uint32) {
	delete(liveBuffers, ptr)
}

//go:wasmexport vbx_abi_version
func vbxABIVersion() uint32 {
	return 1
}

// ------------------------------------------------------------
// JSON-Wire
// ------------------------------------------------------------

type jsonValue struct {
	Type    string               `json:"type"`
	Num     float64              `json:"num,omitempty"`
	Str     string               `json:"str,omitempty"`
	Bool    bool                 `json:"bool,omitempty"`
	Arr     []jsonValue          `json:"arr,omitempty"`
	Arr2D   [][]jsonValue        `json:"arr2d,omitempty"`
	Map     map[string]jsonValue `json:"map,omitempty"`
	Bytes   []byte               `json:"bytes,omitempty"`
	Message string               `json:"message,omitempty"`
}

type funcDesc struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Params      string `json:"params"`
	Description string `json:"description"`
}

func packBytes(data []byte) uint64 {
	ptr := alloc(uint32(len(data)))
	copy(liveBuffers[ptr], data)

	return (uint64(ptr) << 32) | uint64(len(data))
}

func readBytes(ptr, length uint32) []byte {
	buf, ok := liveBuffers[ptr]
	if !ok {
		return nil
	}

	if uint32(len(buf)) < length {
		return buf
	}

	return buf[:length]
}

func errorResult(msg string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type:    "error",
		Message: msg,
	})
	return data
}

func strResult(s string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "str",
		Str:  s,
	})
	return data
}

func boolResult(v bool) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "bool",
		Bool: v,
	})
	return data
}

func numResult(v float64) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "num",
		Num:  v,
	})
	return data
}

func arrayResult(values []jsonValue) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "arr",
		Arr:  values,
	})
	return data
}

// ------------------------------------------------------------
// Argument-Helfer
// ------------------------------------------------------------

func getStringArg(args []jsonValue, idx int, funcName string, required bool) (string, error) {
	if len(args) <= idx {
		if required {
			return "", fmt.Errorf("%s: Argument %d fehlt", funcName, idx+1)
		}
		return "", nil
	}

	if args[idx].Type != "str" {
		return "", fmt.Errorf("%s: Argument %d muss ein String sein", funcName, idx+1)
	}

	return args[idx].Str, nil
}

func getNumArg(args []jsonValue, idx int, funcName string, defaultValue float64) (float64, error) {
	if len(args) <= idx {
		return defaultValue, nil
	}

	if args[idx].Type != "num" {
		return 0, fmt.Errorf("%s: Argument %d muss eine Zahl sein", funcName, idx+1)
	}

	return args[idx].Num, nil
}

func getBoolArg(args []jsonValue, idx int, funcName string, defaultValue bool) (bool, error) {
	if len(args) <= idx {
		return defaultValue, nil
	}

	if args[idx].Type != "bool" {
		return false, fmt.Errorf("%s: Argument %d muss ein Boolean sein", funcName, idx+1)
	}

	return args[idx].Bool, nil
}

// ------------------------------------------------------------
// Beschreibung / Aufruf
// ------------------------------------------------------------

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {
	descs := []funcDesc{
		{
			Namespace:   pluginNamespace,
			Name:        "ReadText",
			Params:      "pfad [, tabellenMarker] [, markdown]",
			Description: "Liest den Text eines DOCX- oder ODT-Dokuments.",
		},
		{
			Namespace:   pluginNamespace,
			Name:        "Convert",
			Params:      "quelle, ziel",
			Description: "Konvertiert ein Dokument zwischen DOCX und ODT (Richtung ergibt sich aus den Endungen).",
		},
		{
			Namespace:   pluginNamespace,
			Name:        "Tables",
			Params:      "pfad",
			Description: "Liest alle Tabellen eines DOCX- oder ODT-Dokuments.",
		},
		{
			Namespace:   pluginNamespace,
			Name:        "New",
			Params:      "",
			Description: "Erzeugt ein neues leeres Dokument.",
		},
		{
			Namespace:   pluginNamespace,
			Name:        "Font",
			Params:      "name [, size]",
			Description: "Setzt Schriftart und optional Schriftgröße des aktuellen Dokuments.",
		},
		{
			Namespace:   pluginNamespace,
			Name:        "Heading",
			Params:      "text, level",
			Description: "Fügt eine Überschrift der Ebene 1 bis 3 hinzu.",
		},
		{
			Namespace:   pluginNamespace,
			Name:        "Text",
			Params:      "text [, bold] [, italic] [, underline]",
			Description: "Fügt einen formatierten Absatz hinzu.",
		},
		{
			Namespace:   pluginNamespace,
			Name:        "List",
			Params:      "array [, numbered]",
			Description: "Fügt eine Aufzählung oder nummerierte Liste hinzu.",
		},
		{
			Namespace:   pluginNamespace,
			Name:        "Table",
			Params:      "array [, header]",
			Description: "Fügt eine Tabelle hinzu.",
		},
		{
			Namespace:   pluginNamespace,
			Name:        "PageBreak",
			Params:      "",
			Description: "Fügt einen Seitenumbruch hinzu.",
		},
		{
			Namespace:   pluginNamespace,
			Name:        "Save",
			Params:      "pfad",
			Description: "Speichert das aktuelle Dokument als DOCX oder ODT.",
		},
		{
			Namespace:   pluginNamespace,
			Name:        "Write",
			Params:      "pfad, text",
			Description: "Schreibt Text direkt als neues DOCX- oder ODT-Dokument.",
		},
	}

	data, _ := json.Marshal(descs)

	return packBytes(data)
}

//go:wasmexport vbx_call
func vbxCall(namePtr, nameLen, argsPtr, argsLen uint32) uint64 {
	nameBytes := readBytes(namePtr, nameLen)
	argsBytes := readBytes(argsPtr, argsLen)

	if nameBytes == nil {
		return packBytes(errorResult("doc: Funktionsname konnte nicht gelesen werden"))
	}

	var args []jsonValue

	if argsLen > 0 {
		if argsBytes == nil {
			return packBytes(errorResult("doc: Argumente konnten nicht gelesen werden"))
		}

		if err := json.Unmarshal(argsBytes, &args); err != nil {
			return packBytes(errorResult(fmt.Sprintf(
				"doc: Argumente konnten nicht gelesen werden: %v",
				err,
			)))
		}
	}

	name := string(nameBytes)

	var result []byte

	switch name {
	case "ReadText":
		result = handleReadText(args)

	case "Convert":
		result = handleConvert(args)

	case "Tables":
		result = handleTables(args)

	case "New":
		result = handleNew(args)

	case "Font":
		result = handleFont(args)

	case "Heading":
		result = handleHeading(args)

	case "Text":
		result = handleText(args)

	case "List":
		result = handleAddList(args)

	case "Table":
		result = handleAddTable(args)

	case "PageBreak":
		result = handlePageBreak(args)

	case "Save":
		result = handleSave(args)

	case "Write":
		result = handleWrite(args)

	default:
		result = errorResult(fmt.Sprintf(
			"doc: Unbekannte Funktion '%s'",
			name,
		))
	}

	return packBytes(result)
}

// ------------------------------------------------------------
// Lesen: DOCX / ODT
// ------------------------------------------------------------

func handleReadText(args []jsonValue) []byte {
	const fn = "doc.ReadText"

	path, err := getStringArg(args, 0, fn, true)
	if err != nil {
		return errorResult(err.Error())
	}

	markers := false
	if len(args) > 1 {
		if args[1].Type != "bool" {
			return errorResult(fn + ": Argument 2 muss ein Boolean sein")
		}
		markers = args[1].Bool
	}

	markdown := false
	if len(args) > 2 {
		if args[2].Type != "bool" {
			return errorResult(fn + ": Argument 3 muss ein Boolean sein")
		}
		markdown = args[2].Bool
	}

	if len(args) > 3 {
		return errorResult(fn + ": erwartet maximal 3 Argumente")
	}

	if markdown {
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".docx" && ext != ".odt" {
			return errorResult(
				fmt.Sprintf(
					"%s: Markdown-Ausgabe unterstützt nur .docx und .odt: %s",
					fn,
					path,
				),
			)
		}

		md, warnings, err := tabula.Open(path).ToMarkdown()
		if err != nil {
			return errorResult(fmt.Sprintf("%s: %v", fn, err))
		}

		_ = warnings
		return strResult(md)
	}

	// Bisheriger ReadText-Code
	blocks, err := readDoc(path)
	if err != nil {
		return errorResult(fmt.Sprintf("%s: %v", fn, err))
	}

	lines := make([]string, 0, len(blocks))
	nTab := 0

	for _, b := range blocks {
		if b.Table != nil {
			nTab++

			if markers {
				lines = append(lines, fmt.Sprintf("[Tabelle %d]", nTab))
			}

			continue
		}

		lines = append(lines, b.Text)
	}

	return strResult(strings.Join(lines, "\n"))
}

func handleTables(args []jsonValue) []byte {
	const fn = "doc.Tables"

	path, err := getStringArg(args, 0, fn, true)
	if err != nil {
		return errorResult(err.Error())
	}

	blocks, err := readDoc(path)
	if err != nil {
		return errorResult(fmt.Sprintf("%s: %v", fn, err))
	}

	tables := make([]jsonValue, 0)

	for _, b := range blocks {
		if b.Table == nil {
			continue
		}

		width := 0

		for _, row := range b.Table {
			if len(row) > width {
				width = len(row)
			}
		}

		grid := make([][]jsonValue, len(b.Table))

		for i, row := range b.Table {
			grid[i] = make([]jsonValue, width)

			for j := 0; j < width; j++ {
				cell := ""

				if j < len(row) {
					cell = row[j]
				}

				grid[i][j] = jsonValue{
					Type: "str",
					Str:  cell,
				}
			}
		}

		tables = append(tables, jsonValue{
			Type:  "arr2d",
			Arr2D: grid,
		})
	}

	return arrayResult(tables)
}

type Block struct {
	Text  string
	Table [][]string
}

func readDoc(path string) ([]Block, error) {
	odt := false
	name := "word/document.xml"

	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx":

	case ".odt":
		odt = true
		name = "content.xml"

	default:
		return nil, fmt.Errorf(
			"nur .docx und .odt werden unterstützt: %s",
			path,
		)
	}

	absP, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf(
			"pfad '%s' konnte nicht aufgelöst werden: %w",
			path,
			err,
		)
	}

	zr, err := zip.OpenReader(absP)
	if err != nil {
		return nil, fmt.Errorf(
			"datei konnte nicht geöffnet werden: %w",
			err,
		)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != name {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()

		return parse(io.LimitReader(rc, maxXMLBytes), odt)
	}

	return nil, fmt.Errorf(
		"%s nicht im Archiv gefunden (keine gültige Datei?)",
		name,
	)
}

func attr(e xml.StartElement, local string) string {
	for _, a := range e.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}

	return ""
}

func parse(r io.Reader, odt bool) ([]Block, error) {
	dec := xml.NewDecoder(r)

	var (
		blocks []Block
		para   strings.Builder
		stack  []string

		paraDepth int
		tblDepth  int
		inT       bool

		rows [][]string
		row  []string
		cell []string
	)

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

			case "t":
				inT = true

			case "tbl", "table":
				tblDepth++

				if tblDepth == 1 {
					rows = [][]string{}
				}

			case "tr", "table-row":
				if tblDepth == 1 {
					row = nil
				}

			case "tc", "table-cell", "covered-table-cell":
				if tblDepth == 1 {
					cell = nil
				}

			case "tab":
				if paraDepth > 0 && parent != "tabs" {
					para.WriteByte('\t')
				}

			case "br", "line-break":
				if paraDepth > 0 && attr(t, "type") != "page" {
					para.WriteByte('\n')
				}

			case "s":
				if odt && paraDepth > 0 {
					c := 1

					if v := attr(t, "c"); v != "" {
						fmt.Sscan(v, &c)
					}

					if c < 0 {
						c = 0
					}

					para.WriteString(strings.Repeat(" ", c))
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

			case "p", "h":
				paraDepth--

				if paraDepth == 0 {
					text := para.String()
					para.Reset()

					if tblDepth > 0 {
						cell = append(cell, text)
					} else {
						blocks = append(blocks, Block{
							Text: text,
						})
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
				if tblDepth == 1 {
					blocks = append(blocks, Block{
						Table: rows,
					})
				}

				tblDepth--
			}

		case xml.CharData:
			if paraDepth > 0 && (odt || inT) {
				para.Write(t)
			}
		}
	}

	return blocks, nil
}

// ------------------------------------------------------------
// Schreiben: Dokument-Zustand
// ------------------------------------------------------------

type docBlock struct {
	kind                    string // "h", "p", "list", "table", "pb"
	level                   int
	text                    string
	items                   []string
	numbered                bool
	rows                    [][]string
	header                  bool
	bold, italic, underline bool
}

type docState struct {
	font   string
	size   float64
	blocks []docBlock
}

var cur *docState

func newDocState() *docState {
	return &docState{
		font: "Arial",
		size: 11,
	}
}

func curDoc() *docState {
	if cur == nil {
		cur = newDocState()
	}

	return cur
}

// ------------------------------------------------------------
// Schreiben: Helfer
// ------------------------------------------------------------

var xmlEsc = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"\"", "&quot;",
)

var fontClean = strings.NewReplacer(
	"'", "",
	"\"", "",
	"<", "",
	">", "",
	"&", "",
)

func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' {
			return r
		}

		if r < 0x20 || r == 0xFFFE || r == 0xFFFF {
			return -1
		}

		return r
	}, s)
}

func valueText(v jsonValue) string {
	switch v.Type {
	case "str":
		return cleanText(v.Str)

	case "num":
		return strconv.FormatFloat(v.Num, 'f', -1, 64)

	case "bool":
		if v.Bool {
			return "True"
		}

		return "False"
	}

	return ""
}

func getTextArg(args []jsonValue, idx int, funcName string) (string, error) {
	if len(args) <= idx {
		return "", fmt.Errorf(
			"%s: Argument %d fehlt",
			funcName,
			idx+1,
		)
	}

	switch args[idx].Type {
	case "str", "num", "bool":
		return valueText(args[idx]), nil

	case "null", "empty":
		return "", nil
	}

	return "", fmt.Errorf(
		"%s: Argument %d muss ein Text sein",
		funcName,
		idx+1,
	)
}

func tableRows(v jsonValue) ([][]string, error) {
	var rows [][]string

	switch v.Type {
	case "arr2d":
		for _, r := range v.Arr2D {
			row := make([]string, len(r))

			for j, c := range r {
				row[j] = valueText(c)
			}

			rows = append(rows, row)
		}

	case "arr":
		nested := false

		for _, e := range v.Arr {
			if e.Type == "arr" {
				nested = true
				break
			}
		}

		if !nested {
			// Einfaches Array = eine Zeile.
			row := make([]string, len(v.Arr))

			for j, c := range v.Arr {
				row[j] = valueText(c)
			}

			rows = append(rows, row)
		} else {
			for _, e := range v.Arr {
				if e.Type == "arr" {
					row := make([]string, len(e.Arr))

					for j, c := range e.Arr {
						row[j] = valueText(c)
					}

					rows = append(rows, row)
				} else {
					rows = append(rows, []string{
						valueText(e),
					})
				}
			}
		}

	default:
		return nil, fmt.Errorf(
			"Argument 1 muss ein Array sein",
		)
	}

	width := 0

	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}

	if len(rows) == 0 || width == 0 {
		return nil, fmt.Errorf(
			"die Tabelle ist leer",
		)
	}

	for i := range rows {
		for len(rows[i]) < width {
			rows[i] = append(rows[i], "")
		}
	}

	return rows, nil
}

// ------------------------------------------------------------
// Schreiben: Handler
// ------------------------------------------------------------

func handleNew(args []jsonValue) []byte {
	cur = newDocState()
	return boolResult(true)
}

func handleFont(args []jsonValue) []byte {
	const fn = "doc.Font"

	name, err := getStringArg(args, 0, fn, true)
	if err != nil {
		return errorResult(err.Error())
	}

	name = strings.Join(
		strings.Fields(
			fontClean.Replace(cleanText(name)),
		),
		" ",
	)

	if name == "" {
		return errorResult(fn + ": Schriftname ist leer")
	}

	if len(name) > 100 {
		return errorResult(fn + ": Schriftname ist zu lang")
	}

	size, err := getNumArg(args, 1, fn, 0)
	if err != nil {
		return errorResult(err.Error())
	}

	if size != 0 && (size < 4 || size > 200) {
		return errorResult(
			fn + ": Schriftgröße muss zwischen 4 und 200 pt liegen",
		)
	}

	d := curDoc()

	d.font = name

	if size != 0 {
		d.size = size
	}

	return boolResult(true)
}

func handleHeading(args []jsonValue) []byte {
	const fn = "doc.Heading"

	text, err := getTextArg(args, 0, fn)
	if err != nil {
		return errorResult(err.Error())
	}

	lv, err := getNumArg(args, 1, fn, 1)
	if err != nil {
		return errorResult(err.Error())
	}

	level := int(lv)

	if float64(level) != lv || level < 1 || level > 3 {
		return errorResult(
			fn + ": Ebene muss 1, 2 oder 3 sein",
		)
	}

	d := curDoc()

	d.blocks = append(d.blocks, docBlock{
		kind:  "h",
		level: level,
		text:  text,
	})

	return boolResult(true)
}

func handleText(args []jsonValue) []byte {
	const fn = "doc.Text"

	text, err := getTextArg(args, 0, fn)
	if err != nil {
		return errorResult(err.Error())
	}

	bold, err := getBoolArg(args, 1, fn, false)
	if err != nil {
		return errorResult(err.Error())
	}

	italic, err := getBoolArg(args, 2, fn, false)
	if err != nil {
		return errorResult(err.Error())
	}

	underline, err := getBoolArg(args, 3, fn, false)
	if err != nil {
		return errorResult(err.Error())
	}

	d := curDoc()

	d.blocks = append(d.blocks, docBlock{
		kind:      "p",
		text:      text,
		bold:      bold,
		italic:    italic,
		underline: underline,
	})

	return boolResult(true)
}

func handleAddList(args []jsonValue) []byte {
	const fn = "doc.List"

	if len(args) < 1 || args[0].Type != "arr" {
		return errorResult(
			fn + ": Argument 1 muss ein Array sein",
		)
	}

	numbered, err := getBoolArg(args, 1, fn, false)
	if err != nil {
		return errorResult(err.Error())
	}

	items := make([]string, 0, len(args[0].Arr))

	for _, e := range args[0].Arr {
		items = append(items, valueText(e))
	}

	if len(items) == 0 {
		return errorResult(
			fn + ": keine Einträge angegeben",
		)
	}

	d := curDoc()

	d.blocks = append(d.blocks, docBlock{
		kind:     "list",
		items:    items,
		numbered: numbered,
	})

	return boolResult(true)
}

func handleAddTable(args []jsonValue) []byte {
	const fn = "doc.Table"

	if len(args) < 1 {
		return errorResult(
			fn + ": Argument 1 fehlt",
		)
	}

	rows, err := tableRows(args[0])
	if err != nil {
		return errorResult(
			fn + ": " + err.Error(),
		)
	}

	header, err := getBoolArg(args, 1, fn, true)
	if err != nil {
		return errorResult(err.Error())
	}

	d := curDoc()

	d.blocks = append(d.blocks, docBlock{
		kind:   "table",
		rows:   rows,
		header: header,
	})

	return boolResult(true)
}

func handlePageBreak(args []jsonValue) []byte {
	const fn = "doc.PageBreak"

	if len(args) != 0 {
		return errorResult(
			fn + ": erwartet keine Argumente",
		)
	}

	d := curDoc()

	d.blocks = append(d.blocks, docBlock{
		kind: "pb",
	})

	return boolResult(true)
}

// ------------------------------------------------------------
// Speichern
// ------------------------------------------------------------

func handleSave(args []jsonValue) []byte {
	const fn = "doc.Save"

	path, err := getStringArg(args, 0, fn, true)
	if err != nil {
		return errorResult(err.Error())
	}

	if err := saveDoc(curDoc(), path); err != nil {
		return errorResult(fmt.Sprintf(
			"%s: %v",
			fn,
			err,
		))
	}

	return boolResult(true)
}

func handleWrite(args []jsonValue) []byte {
	const fn = "doc.Write"

	path, err := getStringArg(args, 0, fn, true)
	if err != nil {
		return errorResult(err.Error())
	}

	text, err := getTextArg(args, 1, fn)
	if err != nil {
		return errorResult(err.Error())
	}

	d := newDocState()

	for _, p := range splitParagraphs(text) {
		d.blocks = append(d.blocks, docBlock{
			kind: "p",
			text: p,
		})
	}

	if err := saveDoc(d, path); err != nil {
		return errorResult(fmt.Sprintf(
			"%s: %v",
			fn,
			err,
		))
	}

	return boolResult(true)
}

// splitParagraphs trennt an Leerzeilen.
// Einzelne Zeilenumbrüche bleiben als Zeilenumbruch innerhalb
// des Absatzes erhalten.
func splitParagraphs(text string) []string {
	var paras []string
	var curLines []string

	flush := func() {
		if len(curLines) > 0 {
			paras = append(
				paras,
				strings.Join(curLines, "\n"),
			)

			curLines = nil
		}
	}

	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}

		curLines = append(curLines, line)
	}

	flush()

	return paras
}

func saveDoc(d *docState, path string) error {
	if len(d.blocks) == 0 {
		return fmt.Errorf("das Dokument ist leer")
	}

	var (
		data []byte
		err  error
	)

	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx":
		data, err = buildDocx(d)

	case ".odt":
		data, err = buildOdt(d)

	default:
		return fmt.Errorf(
			"nur .docx und .odt werden unterstützt: %s",
			path,
		)
	}

	if err != nil {
		return err
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf(
			"pfad '%s' konnte nicht aufgelöst werden: %w",
			path,
			err,
		)
	}

	// Atomar schreiben:
	// erst .tmp, danach umbenennen.
	tmp := abs + ".tmp"

	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf(
			"datei konnte nicht geschrieben werden: %w",
			err,
		)
	}

	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf(
			"umbenennen fehlgeschlagen: %w",
			err,
		)
	}

	return nil
}

// ------------------------------------------------------------
// ZIP-Container
// ------------------------------------------------------------

type zipEntry struct {
	name  string
	data  string
	store bool
}

func buildZip(files []zipEntry) ([]byte, error) {
	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)
	now := time.Now()

	for _, f := range files {
		data := []byte(f.data)

		if f.store {
			// ODF-"mimetype": unkomprimiert, ohne Zusatzfelder und ohne
			// Data-Descriptor. CreateHeader setzt immer einen Data-Descriptor
			// (Größen/CRC stehen dann erst NACH den Daten) - CreateRaw nicht:
			// hier stehen CRC und Größen schon im Header.
			d, t := dosDateTime(now)

			w, err := zw.CreateRaw(&zip.FileHeader{
				Name:               f.name,
				Method:             zip.Store,
				CreatorVersion:     20,
				ReaderVersion:      20,
				CRC32:              crc32.ChecksumIEEE(data),
				CompressedSize64:   uint64(len(data)),
				UncompressedSize64: uint64(len(data)),
				ModifiedDate:       d,
				ModifiedTime:       t,
			})
			if err != nil {
				return nil, err
			}

			if _, err := w.Write(data); err != nil {
				return nil, err
			}

			continue
		}

		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     f.name,
			Method:   zip.Deflate,
			Modified: now,
		})
		if err != nil {
			return nil, err
		}

		if _, err := w.Write(data); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// ------------------------------------------------------------
// DOCX
// ------------------------------------------------------------

const xmlDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

const wNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`

const docxContentTypes = xmlDecl +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	`<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>` +
	`</Types>`

const docxRootRels = xmlDecl +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`</Relationships>`

const docxDocRels = xmlDecl +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>` +
	`</Relationships>`

const docxAbstractNums = `<w:abstractNum w:abstractNumId="0"><w:multiLevelType w:val="hybridMultilevel"/>` +
	`<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/><w:lvlJc w:val="left"/>` +
	`<w:pPr><w:ind w:left="720" w:hanging="360"/></w:pPr>` +
	`<w:rPr><w:rFonts w:ascii="Arial" w:hAnsi="Arial" w:cs="Arial" w:hint="default"/></w:rPr></w:lvl></w:abstractNum>` +
	`<w:abstractNum w:abstractNumId="1"><w:multiLevelType w:val="hybridMultilevel"/>` +
	`<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/><w:lvlJc w:val="left"/>` +
	`<w:pPr><w:ind w:left="720" w:hanging="360"/></w:pPr></w:lvl></w:abstractNum>`

func buildDocx(d *docState) ([]byte, error) {
	var body strings.Builder
	numberedLists := 0

	for _, bl := range d.blocks {
		switch bl.kind {
		case "h":
			body.WriteString(
				docxPara(
					bl.text,
					fmt.Sprintf(
						`<w:pStyle w:val="Heading%d"/>`,
						bl.level,
					),
					"",
				),
			)

		case "p":
			body.WriteString(
				docxPara(
					bl.text,
					"",
					docxRPr(
						bl.bold,
						bl.italic,
						bl.underline,
					),
				),
			)

		case "list":
			numID := 1

			if bl.numbered {
				numberedLists++
				numID = numberedLists + 1
			}

			ppr := fmt.Sprintf(
				`<w:numPr><w:ilvl w:val="0"/><w:numId w:val="%d"/></w:numPr><w:spacing w:after="40"/>`,
				numID,
			)

			for _, it := range bl.items {
				body.WriteString(
					docxPara(it, ppr, ""),
				)
			}

		case "table":
			body.WriteString(docxTable(bl))
			body.WriteString("<w:p/>")

		case "pb":
			body.WriteString(
				`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`,
			)
		}
	}

	docXML := xmlDecl +
		`<w:document ` + wNS + `><w:body>` +
		body.String() +
		`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/>` +
		`<w:pgMar w:top="1417" w:right="1417" w:bottom="1134" w:left="1417" w:header="708" w:footer="708" w:gutter="0"/>` +
		`</w:sectPr></w:body></w:document>`

	return buildZip([]zipEntry{
		{
			name: "[Content_Types].xml",
			data: docxContentTypes,
		},
		{
			name: "_rels/.rels",
			data: docxRootRels,
		},
		{
			name: "word/document.xml",
			data: docXML,
		},
		{
			name: "word/styles.xml",
			data: docxStyles(d),
		},
		{
			name: "word/numbering.xml",
			data: docxNumbering(numberedLists),
		},
		{
			name: "word/_rels/document.xml.rels",
			data: docxDocRels,
		},
	})
}

func docxRPr(bold, italic, underline bool) string {
	s := ""

	if bold {
		s += "<w:b/><w:bCs/>"
	}

	if italic {
		s += "<w:i/><w:iCs/>"
	}

	if underline {
		s += `<w:u w:val="single"/>`
	}

	return s
}

// docxRun erzeugt einen Run.
// \n wird zu <w:br/>, \t zu <w:tab/>.
func docxRun(text, rpr string) string {
	var b strings.Builder
	var seg strings.Builder

	b.WriteString("<w:r>")

	if rpr != "" {
		b.WriteString(
			"<w:rPr>" + rpr + "</w:rPr>",
		)
	}

	flush := func() {
		if seg.Len() > 0 {
			b.WriteString(
				`<w:t xml:space="preserve">` +
					xmlEsc.Replace(seg.String()) +
					`</w:t>`,
			)

			seg.Reset()
		}
	}

	for _, r := range text {
		switch r {
		case '\n':
			flush()
			b.WriteString("<w:br/>")

		case '\t':
			flush()
			b.WriteString("<w:tab/>")

		default:
			seg.WriteRune(r)
		}
	}

	flush()

	b.WriteString("</w:r>")

	return b.String()
}

func docxPara(text, ppr, rpr string) string {
	var b strings.Builder

	b.WriteString("<w:p>")

	if ppr != "" {
		b.WriteString(
			"<w:pPr>" + ppr + "</w:pPr>",
		)
	}

	if text != "" {
		b.WriteString(
			docxRun(text, rpr),
		)
	}

	b.WriteString("</w:p>")

	return b.String()
}

func docxTable(bl docBlock) string {
	ncols := len(bl.rows[0])
	colW := 9072 / ncols

	var b strings.Builder

	b.WriteString(
		`<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/><w:tblBorders>`,
	)

	for _, e := range []string{
		"top",
		"left",
		"bottom",
		"right",
		"insideH",
		"insideV",
	} {
		fmt.Fprintf(
			&b,
			`<w:%s w:val="single" w:sz="4" w:space="0" w:color="808080"/>`,
			e,
		)
	}

	b.WriteString(
		`</w:tblBorders><w:tblCellMar>` +
			`<w:top w:w="40" w:type="dxa"/>` +
			`<w:left w:w="100" w:type="dxa"/>` +
			`<w:bottom w:w="40" w:type="dxa"/>` +
			`<w:right w:w="100" w:type="dxa"/>` +
			`</w:tblCellMar></w:tblPr><w:tblGrid>`,
	)

	for i := 0; i < ncols; i++ {
		fmt.Fprintf(
			&b,
			`<w:gridCol w:w="%d"/>`,
			colW,
		)
	}

	b.WriteString(`</w:tblGrid>`)

	for ri, row := range bl.rows {
		isHead := bl.header && ri == 0

		b.WriteString("<w:tr>")

		if isHead {
			b.WriteString(
				`<w:trPr><w:tblHeader/></w:trPr>`,
			)
		}

		for _, cell := range row {
			fmt.Fprintf(
				&b,
				`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>`,
				colW,
			)

			rpr := ""

			if isHead {
				b.WriteString(
					`<w:shd w:val="clear" w:color="auto" w:fill="E7E6E6"/>`,
				)

				rpr = docxRPr(true, false, false)
			}

			b.WriteString(`</w:tcPr>`)

			b.WriteString(
				docxPara(
					cell,
					`<w:spacing w:after="0"/>`,
					rpr,
				),
			)

			b.WriteString("</w:tc>")
		}

		b.WriteString("</w:tr>")
	}

	b.WriteString("</w:tbl>")

	return b.String()
}

func docxStyles(d *docState) string {
	font := xmlEsc.Replace(d.font)
	hp := int(math.Round(d.size * 2))

	var b strings.Builder

	b.WriteString(
		xmlDecl + `<w:styles ` + wNS + `>`,
	)

	fmt.Fprintf(
		&b,
		`<w:docDefaults><w:rPrDefault><w:rPr>`+
			`<w:rFonts w:ascii="%[1]s" w:hAnsi="%[1]s" w:eastAsia="%[1]s" w:cs="%[1]s"/>`+
			`<w:sz w:val="%[2]d"/><w:szCs w:val="%[2]d"/>`+
			`<w:lang w:val="de-DE" w:eastAsia="en-US" w:bidi="ar-SA"/>`+
			`</w:rPr></w:rPrDefault>`+
			`<w:pPrDefault><w:pPr>`+
			`<w:spacing w:after="120" w:line="276" w:lineRule="auto"/>`+
			`</w:pPr></w:pPrDefault></w:docDefaults>`,
		font,
		hp,
	)

	b.WriteString(
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal">` +
			`<w:name w:val="Normal"/><w:qFormat/></w:style>`,
	)

	for i, f := range []float64{
		1.6,
		1.3,
		1.15,
	} {
		sz := int(
			math.Round(d.size * 2 * f),
		)

		fmt.Fprintf(
			&b,
			`<w:style w:type="paragraph" w:styleId="Heading%[1]d">`+
				`<w:name w:val="heading %[1]d"/>`+
				`<w:basedOn w:val="Normal"/><w:next w:val="Normal"/>`+
				`<w:uiPriority w:val="9"/><w:qFormat/>`+
				`<w:pPr><w:keepNext/>`+
				`<w:spacing w:before="%[2]d" w:after="120"/>`+
				`<w:outlineLvl w:val="%[3]d"/></w:pPr>`+
				`<w:rPr><w:b/><w:bCs/>`+
				`<w:sz w:val="%[4]d"/><w:szCs w:val="%[4]d"/>`+
				`</w:rPr></w:style>`,
			i+1,
			360-i*60,
			i,
			sz,
		)
	}

	b.WriteString(
		`<w:style w:type="table" w:default="1" w:styleId="TableNormal">` +
			`<w:name w:val="Normal Table"/>` +
			`<w:uiPriority w:val="99"/><w:semiHidden/><w:unhideWhenUsed/>` +
			`<w:tblPr><w:tblInd w:w="0" w:type="dxa"/>` +
			`<w:tblCellMar>` +
			`<w:top w:w="0" w:type="dxa"/>` +
			`<w:left w:w="108" w:type="dxa"/>` +
			`<w:bottom w:w="0" w:type="dxa"/>` +
			`<w:right w:w="108" w:type="dxa"/>` +
			`</w:tblCellMar></w:tblPr></w:style>`,
	)

	b.WriteString(`</w:styles>`)

	return b.String()
}

func docxNumbering(numbered int) string {
	var b strings.Builder

	b.WriteString(
		xmlDecl + `<w:numbering ` + wNS + `>`,
	)

	b.WriteString(docxAbstractNums)

	b.WriteString(
		`<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>`,
	)

	// Jede nummerierte Liste bekommt eine eigene Nummerierung,
	// die wieder bei 1 startet.
	for i := 0; i < numbered; i++ {
		fmt.Fprintf(
			&b,
			`<w:num w:numId="%d"><w:abstractNumId w:val="1"/>`+
				`<w:lvlOverride w:ilvl="0">`+
				`<w:startOverride w:val="1"/>`+
				`</w:lvlOverride></w:num>`,
			i+2,
		)
	}

	b.WriteString(`</w:numbering>`)

	return b.String()
}

// ------------------------------------------------------------
// ODT
// ------------------------------------------------------------

const odfNS = `xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
	`xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" ` +
	`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
	`xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" ` +
	`xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" ` +
	`xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0"`

const odtXMLDecl = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

const odtMimetype = "application/vnd.oasis.opendocument.text"

const odtManifest = odtXMLDecl +
	`<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">` +
	`<manifest:file-entry manifest:full-path="/" manifest:version="1.2" manifest:media-type="` +
	odtMimetype +
	`"/>` +
	`<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>` +
	`<manifest:file-entry manifest:full-path="styles.xml" manifest:media-type="text/xml"/>` +
	`</manifest:manifest>`

const odtListStyles = `<text:list-style style:name="LBul">` +
	`<text:list-level-style-bullet text:level="1" text:bullet-char="•">` +
	`<style:list-level-properties text:list-level-position-and-space-mode="label-alignment">` +
	`<style:list-level-label-alignment text:label-followed-by="listtab" text:list-tab-stop-position="1.27cm" fo:text-indent="-0.635cm" fo:margin-left="1.27cm"/>` +
	`</style:list-level-properties>` +
	`</text:list-level-style-bullet>` +
	`</text:list-style>` +
	`<text:list-style style:name="LNum">` +
	`<text:list-level-style-number text:level="1" style:num-suffix="." style:num-format="1">` +
	`<style:list-level-properties text:list-level-position-and-space-mode="label-alignment">` +
	`<style:list-level-label-alignment text:label-followed-by="listtab" text:list-tab-stop-position="1.27cm" fo:text-indent="-0.635cm" fo:margin-left="1.27cm"/>` +
	`</style:list-level-properties>` +
	`</text:list-level-style-number>` +
	`</text:list-style>`

const odtFixedStyles = `<style:style style:name="PB" style:family="paragraph" style:parent-style-name="Standard">` +
	`<style:paragraph-properties fo:break-before="page"/>` +
	`</style:style>` +
	`<style:style style:name="TabText" style:family="paragraph" style:parent-style-name="Standard">` +
	`<style:paragraph-properties fo:margin-top="0cm" fo:margin-bottom="0cm"/>` +
	`</style:style>` +
	`<style:style style:name="TabHeadText" style:family="paragraph" style:parent-style-name="Standard">` +
	`<style:paragraph-properties fo:margin-top="0cm" fo:margin-bottom="0cm"/>` +
	`<style:text-properties fo:font-weight="bold"/>` +
	`</style:style>` +
	`<style:style style:name="TabCell" style:family="table-cell">` +
	`<style:table-cell-properties fo:padding="0.1cm" fo:border="0.5pt solid #808080"/>` +
	`</style:style>` +
	`<style:style style:name="TabHead" style:family="table-cell">` +
	`<style:table-cell-properties fo:padding="0.1cm" fo:border="0.5pt solid #808080" fo:background-color="#e7e6e6"/>` +
	`</style:style>`

var odtSpanNames = []string{
	"Tb",
	"Ti",
	"Tu",
	"Tbi",
	"Tbu",
	"Tiu",
	"Tbiu",
}

func odtSpanStyle(bold, italic, underline bool) string {
	if !bold && !italic && !underline {
		return ""
	}

	n := "T"

	if bold {
		n += "b"
	}

	if italic {
		n += "i"
	}

	if underline {
		n += "u"
	}

	return n
}

func odtSpanStyleXML(name string) string {
	flags := name[1:]
	props := ""

	if strings.Contains(flags, "b") {
		props += ` fo:font-weight="bold"`
	}

	if strings.Contains(flags, "i") {
		props += ` fo:font-style="italic"`
	}

	if strings.Contains(flags, "u") {
		props +=
			` style:text-underline-style="solid"` +
				` style:text-underline-width="auto"` +
				` style:text-underline-color="font-color"`
	}

	return `<style:style style:name="` +
		name +
		`" style:family="text"><style:text-properties` +
		props +
		`/></style:style>`
}

// odtInline wandelt Text in ODF-Inhalt:
// \n -> <text:line-break/>
// \t -> <text:tab/>
// Mehrfache Leerzeichen werden explizit dargestellt.
func odtInline(text string) string {
	var b strings.Builder
	var spaces int

	atStart := true

	flushSpaces := func() {
		if spaces == 0 {
			return
		}

		if atStart {
			fmt.Fprintf(
				&b,
				`<text:s text:c="%d"/>`,
				spaces,
			)
		} else {
			b.WriteByte(' ')

			if spaces > 1 {
				fmt.Fprintf(
					&b,
					`<text:s text:c="%d"/>`,
					spaces-1,
				)
			}
		}

		spaces = 0
	}

	for _, r := range text {
		if r == ' ' {
			spaces++
			continue
		}

		flushSpaces()

		switch r {
		case '\n':
			b.WriteString("<text:line-break/>")
			atStart = true

		case '\t':
			b.WriteString("<text:tab/>")
			atStart = true

		default:
			b.WriteString(
				xmlEsc.Replace(string(r)),
			)

			atStart = false
		}
	}

	flushSpaces()

	return b.String()
}

func buildOdt(d *docState) ([]byte, error) {
	usedSpans := map[string]bool{}

	var body strings.Builder
	var tableStyles strings.Builder

	tableNo := 0

	for _, bl := range d.blocks {
		switch bl.kind {

		case "h":
			fmt.Fprintf(
				&body,
				`<text:h text:style-name="Heading_20_%d" text:outline-level="%d">%s</text:h>`,
				bl.level,
				bl.level,
				odtInline(bl.text),
			)

		case "p":
			inner := odtInline(bl.text)

			if name := odtSpanStyle(
				bl.bold,
				bl.italic,
				bl.underline,
			); name != "" && inner != "" {
				usedSpans[name] = true

				inner =
					`<text:span text:style-name="` +
						name +
						`">` +
						inner +
						`</text:span>`
			}

			body.WriteString(
				`<text:p text:style-name="Standard">` +
					inner +
					`</text:p>`,
			)

		case "list":
			ls := "LBul"

			if bl.numbered {
				ls = "LNum"
			}

			body.WriteString(
				`<text:list text:style-name="` +
					ls +
					`">`,
			)

			for _, it := range bl.items {
				body.WriteString(
					`<text:list-item>` +
						`<text:p text:style-name="Standard">` +
						odtInline(it) +
						`</text:p>` +
						`</text:list-item>`,
				)
			}

			body.WriteString(`</text:list>`)

		case "table":
			if len(bl.rows) == 0 || len(bl.rows[0]) == 0 {
				continue
			}

			tableNo++

			ncols := len(bl.rows[0])

			// Tabellenbreite 16 cm, gleichmäßig auf die Spalten verteilt.
			colWidth := 16.0 / float64(ncols)

			fmt.Fprintf(
				&tableStyles,
				`<style:style style:name="Tb%d" style:family="table">`+
					`<style:table-properties style:width="16cm" table:align="margins"/>`+
					`</style:style>`,
				tableNo,
			)

			fmt.Fprintf(
				&tableStyles,
				`<style:style style:name="Co%d" style:family="table-column">`+
					`<style:table-column-properties style:column-width="%.3fcm"/>`+
					`</style:style>`,
				tableNo,
				colWidth,
			)

			fmt.Fprintf(
				&body,
				`<table:table table:name="Tabelle%d" table:style-name="Tb%d">`,
				tableNo,
				tableNo,
			)

			// Spalten definieren.
			fmt.Fprintf(
				&body,
				`<table:table-column table:style-name="Co%d" table:number-columns-repeated="%d"/>`,
				tableNo,
				ncols,
			)

			// Kopfzeile muss als zusammenhängender Block vor
			// den normalen Tabellenzeilen stehen.
			startRow := 0

			if bl.header && len(bl.rows) > 0 {
				body.WriteString(`<table:table-header-rows>`)
				body.WriteString(`<table:table-row>`)

				for _, cell := range bl.rows[0] {
					body.WriteString(
						`<table:table-cell table:style-name="TabHead" office:value-type="string">` +
							`<text:p text:style-name="TabHeadText">` +
							odtInline(cell) +
							`</text:p>` +
							`</table:table-cell>`,
					)
				}

				body.WriteString(`</table:table-row>`)
				body.WriteString(`</table:table-header-rows>`)

				startRow = 1
			}

			// Normale Tabellenzeilen.
			for ri := startRow; ri < len(bl.rows); ri++ {
				row := bl.rows[ri]

				body.WriteString(`<table:table-row>`)

				for _, cell := range row {
					body.WriteString(
						`<table:table-cell table:style-name="TabCell" office:value-type="string">` +
							`<text:p text:style-name="TabText">` +
							odtInline(cell) +
							`</text:p>` +
							`</table:table-cell>`,
					)
				}

				body.WriteString(`</table:table-row>`)
			}

			body.WriteString(
				`</table:table>` +
					`<text:p text:style-name="Standard"/>`,
			)

		case "pb":
			body.WriteString(
				`<text:p text:style-name="PB"/>`,
			)
		}
	}

	var spanStyles strings.Builder

	for _, n := range odtSpanNames {
		if usedSpans[n] {
			spanStyles.WriteString(
				odtSpanStyleXML(n),
			)
		}
	}

	content :=
		odtXMLDecl +
			`<office:document-content ` +
			odfNS +
			` office:version="1.2">` +
			`<office:automatic-styles>` +
			odtFixedStyles +
			odtListStyles +
			spanStyles.String() +
			tableStyles.String() +
			`</office:automatic-styles>` +
			`<office:body><office:text>` +
			body.String() +
			`</office:text></office:body>` +
			`</office:document-content>`

	return buildZip([]zipEntry{
		{
			// Muss erstes ZIP-Element und unkomprimiert sein.
			name:  "mimetype",
			data:  odtMimetype,
			store: true,
		},
		{
			name: "META-INF/manifest.xml",
			data: odtManifest,
		},
		{
			name: "content.xml",
			data: content,
		},
		{
			name: "styles.xml",
			data: odtStyles(d),
		},
	})
}

func odtStyles(d *docState) string {
	font := xmlEsc.Replace(d.font)
	size := strconv.FormatFloat(
		d.size,
		'f',
		-1,
		64,
	) + "pt"

	var b strings.Builder

	b.WriteString(
		odtXMLDecl +
			`<office:document-styles ` +
			odfNS +
			` office:version="1.2">`,
	)

	b.WriteString(
		`<office:font-face-decls>` +
			`<style:font-face style:name="` +
			font +
			`" svg:font-family="'` +
			font +
			`'"/>` +
			`</office:font-face-decls>`,
	)

	b.WriteString(`<office:styles>`)

	b.WriteString(
		`<style:default-style style:family="paragraph">` +
			`<style:paragraph-properties fo:orphans="2" fo:widows="2"/>` +
			`<style:text-properties style:font-name="` +
			font +
			`" fo:font-size="` +
			size +
			`" fo:language="de" fo:country="DE"/>` +
			`</style:default-style>`,
	)

	b.WriteString(
		`<style:style style:name="Standard" style:family="paragraph" style:class="text">` +
			`<style:paragraph-properties fo:margin-top="0cm" fo:margin-bottom="0.21cm" fo:line-height="115%"/>` +
			`</style:style>`,
	)

	for i, f := range []float64{
		1.6,
		1.3,
		1.15,
	} {
		fmt.Fprintf(
			&b,
			`<style:style style:name="Heading_20_%[1]d" `+
				`style:display-name="Heading %[1]d" `+
				`style:family="paragraph" `+
				`style:parent-style-name="Standard" `+
				`style:next-style-name="Standard" `+
				`style:default-outline-level="%[1]d" `+
				`style:class="text">`+
				`<style:paragraph-properties `+
				`fo:margin-top="0.4cm" `+
				`fo:margin-bottom="0.2cm" `+
				`fo:keep-with-next="always"/>`+
				`<style:text-properties `+
				`fo:font-size="%[2].1fpt" `+
				`fo:font-weight="bold"/>`+
				`</style:style>`,
			i+1,
			d.size*f,
		)
	}

	b.WriteString(`</office:styles>`)

	b.WriteString(
		`<office:automatic-styles>` +
			`<style:page-layout style:name="pm1">` +
			`<style:page-layout-properties ` +
			`fo:page-width="21cm" ` +
			`fo:page-height="29.7cm" ` +
			`fo:margin-top="2.5cm" ` +
			`fo:margin-bottom="2cm" ` +
			`fo:margin-left="2.5cm" ` +
			`fo:margin-right="2.5cm"/>` +
			`</style:page-layout>` +
			`</office:automatic-styles>`,
	)

	b.WriteString(
		`<office:master-styles>` +
			`<style:master-page style:name="Standard" style:page-layout-name="pm1"/>` +
			`</office:master-styles>`,
	)

	b.WriteString(`</office:document-styles>`)

	return b.String()
}

// ------------------------------------------------------------
// Main
// ------------------------------------------------------------

func main() {}
