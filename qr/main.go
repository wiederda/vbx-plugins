package main

// ------------------------------------------------------------
// qr.* WASM-Plugin für VBX
//
// Build (normales Go, kein TinyGo):
//   GOOS=wasip1 GOARCH=wasm go build -o qr.wasm .
//
// Abhängigkeiten (aus eurem CLI-Tool übernommen):
//   go get github.com/skip2/go-qrcode
//   go get github.com/liyue201/goqr
//
// Struktur 1:1 an euer fin-Plugin angeglichen (gleiche Helfer-
// namen: alloc/exportAlloc/dealloc, packBytes/readBytes,
// errorResult, requireNum-Analoga), damit es sich nahtlos neben
// die anderen Plugins legt.
// ------------------------------------------------------------

import (
	"encoding/json"
	"image/png"
	"os"
	"unsafe"

	"github.com/liyue201/goqr"
	qrw "github.com/skip2/go-qrcode"
)

// ------------------------------------------------------------
// ABI
// ------------------------------------------------------------

const abiVersion = 1

// ------------------------------------------------------------
// Speicherverwaltung
// ------------------------------------------------------------

var liveBuffers = map[uint32][]byte{}

func alloc(size uint32) uint32 {
	if size == 0 {
		size = 1
	}

	buf := make([]byte, size)
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

// ------------------------------------------------------------
// ABI-Version
// ------------------------------------------------------------

//go:wasmexport vbx_abi_version
func vbxABIVersion() uint32 {
	return abiVersion
}

// ------------------------------------------------------------
// Wire-Format
//
// Muss exakt mit der Host-Seite übereinstimmen.
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

// ------------------------------------------------------------
// Speicher / Rückgabewerte
// ------------------------------------------------------------

func packBytes(data []byte) uint64 {
	ptr := alloc(uint32(len(data)))

	buf, ok := liveBuffers[ptr]
	if !ok {
		return 0
	}

	copy(buf, data)

	return (uint64(ptr) << 32) | uint64(len(data))
}

func readBytes(ptr, length uint32) []byte {
	buf, ok := liveBuffers[ptr]
	if !ok {
		return nil
	}

	if length > uint32(len(buf)) {
		return buf
	}

	return buf[:length]
}

// ------------------------------------------------------------
// JSON-Ergebnisse
// ------------------------------------------------------------

func strResult(s string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "str",
		Str:  s,
	})

	return data
}

func boolResult(b bool) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "bool",
		Bool: b,
	})

	return data
}

func arrResult(vals []jsonValue) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "arr",
		Arr:  vals,
	})

	return data
}

func errorResult(msg string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type:    "error",
		Message: msg,
	})

	return data
}

// ------------------------------------------------------------
// Hilfsfunktionen
// ------------------------------------------------------------

func requireStr(args []jsonValue, index int, function string) (string, []byte) {
	if index >= len(args) {
		return "", errorResult(
			function + ": Argument fehlt",
		)
	}

	if args[index].Type != "str" {
		return "", errorResult(
			function + ": Argument " +
				itoa(index+1) +
				" muss Text sein",
		)
	}

	return args[index].Str, nil
}

// optionalNum liest ein optionales numerisches Argument;
// liefert def, falls das Argument fehlt.
func optionalNum(args []jsonValue, index int, def float64, function string) (float64, []byte) {
	if index >= len(args) {
		return def, nil
	}

	if args[index].Type != "num" {
		return 0, errorResult(
			function + ": Argument " +
				itoa(index+1) +
				" muss eine Zahl sein",
		)
	}

	return args[index].Num, nil
}

// Kleine Integer-Konvertierung ohne zusätzliche Abhängigkeit.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var buf [20]byte
	i := len(buf)

	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}

	return string(buf[i:])
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {
	entries := []funcDesc{
		{
			Namespace:   "qr",
			Name:        "Create",
			Params:      "text, ausgabepfad, [size]",
			Description: "Erstellt einen QR-Code aus text und speichert ihn als PNG unter ausgabepfad. size (Pixelbreite/-höhe, Standard 256) ist optional.",
		},
		{
			Namespace:   "qr",
			Name:        "Read",
			Params:      "pfad",
			Description: "Liest den ersten QR-Code aus der PNG-Datei unter pfad und gibt seinen Inhalt als Text zurück. ErrorVal, falls keiner gefunden wird.",
		},
		{
			Namespace:   "qr",
			Name:        "ReadAll",
			Params:      "pfad",
			Description: "Liest alle QR-Codes aus der PNG-Datei unter pfad und gibt ihre Inhalte als Array zurück.",
		},
	}

	data, _ := json.Marshal(entries)
	return packBytes(data)
}

// ------------------------------------------------------------
// vbx_call
// ------------------------------------------------------------

//go:wasmexport vbx_call
func vbxCall(
	namePtr,
	nameLen,
	argsPtr,
	argsLen uint32,
) uint64 {
	nameBytes := readBytes(namePtr, nameLen)
	argsJSON := readBytes(argsPtr, argsLen)

	if nameBytes == nil {
		return packBytes(errorResult("ungültiger Funktionsname"))
	}

	if argsJSON == nil {
		return packBytes(errorResult("ungültige Argumente"))
	}

	name := string(nameBytes)

	var args []jsonValue

	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return packBytes(
			errorResult("ungültige Argumente: " + err.Error()),
		)
	}

	switch name {
	case "Create":
		return packBytes(handleCreate(args))

	case "Read":
		return packBytes(handleRead(args))

	case "ReadAll":
		return packBytes(handleReadAll(args))

	default:
		return packBytes(
			errorResult("unbekannte Funktion: " + name),
		)
	}
}

// ------------------------------------------------------------
// Create
// ------------------------------------------------------------

func handleCreate(args []jsonValue) []byte {
	if len(args) < 2 {
		return errorResult(
			"Create erwartet mindestens 2 Argumente (text, ausgabepfad)",
		)
	}

	text, err := requireStr(args, 0, "Create")
	if err != nil {
		return err
	}

	path, err := requireStr(args, 1, "Create")
	if err != nil {
		return err
	}

	size, err := optionalNum(args, 2, 256, "Create")
	if err != nil {
		return err
	}

	if size <= 0 {
		return errorResult("Create: size muss größer als 0 sein")
	}

	if writeErr := qrw.WriteFile(text, qrw.Medium, int(size), path); writeErr != nil {
		return errorResult(
			"QR-Code konnte nicht geschrieben werden: " + writeErr.Error(),
		)
	}

	return boolResult(true)
}

// ------------------------------------------------------------
// Read / ReadAll
// ------------------------------------------------------------

func handleRead(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult("Read erwartet 1 Argument (pfad)")
	}

	path, err := requireStr(args, 0, "Read")
	if err != nil {
		return err
	}

	results, decodeErr := decodeQRFile(path)
	if decodeErr != nil {
		return errorResult(decodeErr.Error())
	}

	if len(results) == 0 {
		return errorResult("Kein QR-Code gefunden")
	}

	return strResult(results[0])
}

func handleReadAll(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult("ReadAll erwartet 1 Argument (pfad)")
	}

	path, err := requireStr(args, 0, "ReadAll")
	if err != nil {
		return err
	}

	results, decodeErr := decodeQRFile(path)
	if decodeErr != nil {
		return errorResult(decodeErr.Error())
	}

	vals := make([]jsonValue, len(results))
	for i, r := range results {
		vals[i] = jsonValue{Type: "str", Str: r}
	}

	return arrResult(vals)
}

func decodeQRFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}

	codes, err := goqr.Recognize(img)
	if err != nil {
		return nil, err
	}

	var out []string
	for _, c := range codes {
		out = append(out, string(c.Payload))
	}

	return out, nil
}

// ------------------------------------------------------------
// Reactor entry point
// ------------------------------------------------------------

func main() {}
