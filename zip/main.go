package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
	"unsafe"

	"encoding/json"

	zip "github.com/alexmullins/zip"
	"golang.org/x/text/encoding/charmap"
)

// ------------------------------------------------------------
// Speicherverwaltung
// ------------------------------------------------------------

var liveBuffers = map[uint32][]byte{}

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

func decodeZipName(f *zip.File) string {
	// Bit 11 (0x800) zeigt an, dass der Name bereits UTF-8 ist
	if f.Flags&0x800 != 0 {
		return f.Name
	}

	if utf8.ValidString(f.Name) {
		return f.Name
	}

	// Name ist kein gültiges UTF-8 -> vermutlich CP437 (klassische ZIP-Kodierung)
	decoded, err := charmap.CodePage437.NewDecoder().String(f.Name)
	if err != nil {
		return f.Name
	}

	return decoded
}

// ------------------------------------------------------------
// Wire-Format
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
// Speicher / JSON Helper
// ------------------------------------------------------------

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

func boolResult(value bool) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "bool",
		Bool: value,
	})

	return data
}

func nullResult() []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "null",
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
// Argument Helper
// ------------------------------------------------------------

func getStringArg(
	args []jsonValue,
	idx int,
	funcName string,
	required bool,
) (string, error) {

	if len(args) <= idx {
		if required {
			return "", fmt.Errorf(
				"%s: Argument %d fehlt",
				funcName,
				idx+1,
			)
		}

		return "", nil
	}

	if args[idx].Type != "str" {
		return "", fmt.Errorf(
			"%s: Argument %d muss ein String sein",
			funcName,
			idx+1,
		)
	}

	return args[idx].Str, nil
}

// ------------------------------------------------------------
// Dateien + Passwort aus Argumenten extrahieren
// ------------------------------------------------------------
//
// Unterstützt zwei Aufrufkonventionen, identisch zur nativen
// Implementierung:
//   - (zipPath, [array], [pass])
//   - (zipPath, file1, file2, ..., [pass])
//
// Wichtig: die Pfad-Strings, die hier ankommen, sind bereits
// vom Host (toJSONValue) auf Guest-FS-Pfade übersetzt worden,
// falls sie wie Host-Pfade aussahen — das gilt auch für Strings
// INNERHALB von Arrays, da die Host-Übersetzung rekursiv über
// KindArr läuft. Kein Zusatzaufwand hier nötig.
//

func extractFilesAndPass(
	args []jsonValue,
	funcName string,
) ([]string, string, error) {

	if len(args) >= 2 && args[1].Type == "arr" {

		files := make([]string, 0, len(args[1].Arr))

		for _, el := range args[1].Arr {

			if el.Type != "str" {
				return nil, "", fmt.Errorf(
					"%s: Array-Element ist kein String",
					funcName,
				)
			}

			files = append(files, el.Str)
		}

		pass := ""

		if len(args) >= 3 {

			if args[2].Type != "str" {
				return nil, "", fmt.Errorf(
					"%s: password muss ein String sein",
					funcName,
				)
			}

			pass = args[2].Str
		}

		return files, pass, nil
	}

	lastIdx := len(args) - 1
	pass := ""

	if lastIdx > 1 &&
		args[lastIdx].Type == "str" &&
		!looksLikeFilePath(args[lastIdx].Str) {

		pass = args[lastIdx].Str
		lastIdx--
	}

	var files []string

	for i := 1; i <= lastIdx; i++ {

		if args[i].Type != "str" {
			return nil, "", fmt.Errorf(
				"%s: Argument %d muss ein String sein",
				funcName,
				i+1,
			)
		}

		files = append(files, args[i].Str)
	}

	if len(files) == 0 {
		return nil, "", fmt.Errorf(
			"%s: keine Dateien angegeben",
			funcName,
		)
	}

	return files, pass, nil
}

func looksLikeFilePath(s string) bool {
	return strings.ContainsAny(s, "/\\") ||
		strings.HasPrefix(s, ".") ||
		filepath.IsAbs(s)
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {

	entries := []funcDesc{
		{
			Namespace:   "zip",
			Name:        "Create",
			Params:      "zipPath, files... [, password]",
			Description: "Erstellt ein ZIP-Archiv mit erhaltener Verzeichnisstruktur.",
		},

		{
			Namespace:   "zip",
			Name:        "IsValid",
			Params:      "zipPath",
			Description: "Prüft, ob eine Datei ein gültiges ZIP-Archiv ist (Inhalt, nicht nur Dateiendung).",
		},

		{
			Namespace:   "zip",
			Name:        "CreateFlat",
			Params:      "zipPath, files... [, password]",
			Description: "Erstellt ein ZIP-Archiv ohne Unterordner (alle Dateien auf oberster Ebene).",
		},
		{
			Namespace:   "zip",
			Name:        "Extract",
			Params:      "zipPath, dest [, password]",
			Description: "Entpackt ein Archiv. Schützt gegen Zip-Slip durch Pfad-Validierung.",
		},
		{
			Namespace:   "zip",
			Name:        "Exists",
			Params:      "zipPath, entryName",
			Description: "Prüft, ob eine bestimmte Datei im ZIP-Archiv existiert.",
		},
		{
			Namespace:   "zip",
			Name:        "List",
			Params:      "zipPath",
			Description: "Gibt Details über den Inhalt eines ZIP-Archivs zurück.",
		},
		{
			Namespace:   "zip",
			Name:        "ListNames",
			Params:      "zipPath",
			Description: "Gibt ein Array mit allen Dateinamen im ZIP-Archiv zurück.",
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
		return packBytes(
			errorResult("ungültiger Funktionsname"),
		)
	}

	name := string(nameBytes)

	var args []jsonValue

	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return packBytes(
			errorResult(
				"ungültige Argumente: " + err.Error(),
			),
		)
	}

	switch name {

	case "Create":
		return packBytes(handleCreate(args))

	case "IsValid":
		return packBytes(handleIsValid(args))

	case "CreateFlat":
		return packBytes(handleCreateFlat(args))

	case "Extract":
		return packBytes(handleExtract(args))

	case "Exists":
		return packBytes(handleExists(args))

	case "List":
		return packBytes(handleList(args))

	case "ListNames":
		return packBytes(handleListNames(args))

	default:
		return packBytes(
			errorResult(
				"unbekannte Funktion: " + name,
			),
		)
	}
}

// ------------------------------------------------------------
// Create / CreateFlat
// ------------------------------------------------------------

func handleCreate(args []jsonValue) []byte {
	return handleCreateInternal(args, false, "zip.Create")
}

func handleCreateFlat(args []jsonValue) []byte {
	return handleCreateInternal(args, true, "zip.CreateFlat")
}

func handleCreateInternal(
	args []jsonValue,
	flat bool,
	funcName string,
) []byte {

	if len(args) < 2 {
		return errorResult(
			fmt.Sprintf(
				"%s: Erwartet mindestens zipPath und files.",
				funcName,
			),
		)
	}

	zipPath, err := getStringArg(args, 0, funcName, true)

	if err != nil {
		return errorResult(err.Error())
	}

	files, pass, err := extractFilesAndPass(args, funcName)

	if err != nil {
		return errorResult(err.Error())
	}

	count, err := zipCreate(zipPath, files, pass, flat)

	if err != nil {
		return errorResult(
			fmt.Sprintf(
				"%s: %v",
				funcName,
				err,
			),
		)
	}

	return boolResult(count > 0)
}

// ------------------------------------------------------------
// Extract
// ------------------------------------------------------------

func handleExtract(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult(
			"zip.Extract: Erwartet mindestens zipPath und dest.",
		)
	}

	zipPath, err := getStringArg(args, 0, "zip.Extract", true)

	if err != nil {
		return errorResult(err.Error())
	}

	dest, err := getStringArg(args, 1, "zip.Extract", true)

	if err != nil {
		return errorResult(err.Error())
	}

	pass, err := getStringArg(args, 2, "zip.Extract", false)

	if err != nil {
		return errorResult(err.Error())
	}

	if err := zipExtract(zipPath, dest, pass); err != nil {
		return errorResult(
			fmt.Sprintf(
				"zip.Extract: %v",
				err,
			),
		)
	}

	return nullResult()
}

// ------------------------------------------------------------
// Exists
// ------------------------------------------------------------

func handleExists(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult(
			"zip.Exists: Erwartet zipPath und entryName.",
		)
	}

	zipPath, err := getStringArg(args, 0, "zip.Exists", true)

	if err != nil {
		return errorResult(err.Error())
	}

	entryName, err := getStringArg(args, 1, "zip.Exists", true)

	if err != nil {
		return errorResult(err.Error())
	}

	absP, err := absPathStrict(zipPath)

	if err != nil {
		return errorResult(err.Error())
	}

	r, err := zip.OpenReader(absP)

	if err != nil {
		return errorResult(
			fmt.Sprintf(
				"zip.Exists: Archiv konnte nicht geöffnet werden: %v",
				err,
			),
		)
	}

	defer r.Close()

	entryName = filepath.ToSlash(entryName)

	for _, f := range r.File {
		if decodeZipName(f) == entryName {
			return boolResult(true)
		}
	}

	return boolResult(false)
}

// ------------------------------------------------------------
// List / ListNames
// ------------------------------------------------------------

func handleList(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult(
			"zip.List: Erwartet mindestens zipPath.",
		)
	}

	zipPath, err := getStringArg(args, 0, "zip.List", true)

	if err != nil {
		return errorResult(err.Error())
	}

	absP, err := absPathStrict(zipPath)

	if err != nil {
		return errorResult(err.Error())
	}

	r, err := zip.OpenReader(absP)

	if err != nil {
		return errorResult(
			fmt.Sprintf(
				"zip.List: Archiv konnte nicht geöffnet werden: %v",
				err,
			),
		)
	}

	defer r.Close()

	results := make([]jsonValue, 0, len(r.File))

	for _, f := range r.File {
		results = append(results, jsonValue{
			Type: "map",
			Map: map[string]jsonValue{
				"Name": {
					Type: "str",
					Str:  decodeZipName(f),
				},
				"Size": {
					Type: "num",
					Num:  float64(f.UncompressedSize64),
				},
				"IsDir": {
					Type: "bool",
					Bool: f.FileInfo().IsDir(),
				},
				"ModTime": {
					Type: "str",
					Str:  f.FileInfo().ModTime().Format("2006-01-02 15:04:05"),
				},
			},
		})
	}

	return arrayResult(results)
}

// ------------------------------------------------------------
// IsValid
// ------------------------------------------------------------

func handleIsValid(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult(
			"zip.IsValid: Erwartet zipPath.",
		)
	}

	zipPath, err := getStringArg(args, 0, "zip.IsValid", true)

	if err != nil {
		return errorResult(err.Error())
	}

	absP, err := absPathStrict(zipPath)

	if err != nil {
		return errorResult(err.Error())
	}

	r, err := zip.OpenReader(absP)

	if err != nil {
		return boolResult(false)
	}

	defer r.Close()

	return boolResult(true)
}

func handleListNames(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult(
			"zip.ListNames: Erwartet mindestens zipPath.",
		)
	}

	zipPath, err := getStringArg(args, 0, "zip.ListNames", true)

	if err != nil {
		return errorResult(err.Error())
	}

	absP, err := absPathStrict(zipPath)

	if err != nil {
		return errorResult(err.Error())
	}

	r, err := zip.OpenReader(absP)

	if err != nil {
		return errorResult(
			fmt.Sprintf(
				"zip.ListNames: Archiv konnte nicht geöffnet werden: %v",
				err,
			),
		)
	}

	defer r.Close()

	names := make([]jsonValue, 0, len(r.File))

	for _, f := range r.File {
		names = append(names, jsonValue{
			Type: "str",
			Str:  decodeZipName(f),
		})
	}

	return arrayResult(names)
}

// ------------------------------------------------------------
// ZIP-Logik (1:1 aus stdlib_zip.go übernommen)
// ------------------------------------------------------------

func absPathStrict(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("pfad '%s' konnte nicht aufgelöst werden: %w", p, err)
	}
	return abs, nil
}

func commonBasePath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	cleaned := make([]string, len(paths))
	for i, p := range paths {
		c := filepath.Clean(p)
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			c = filepath.Dir(c)
		}
		cleaned[i] = c
	}

	if len(cleaned) == 1 {
		return cleaned[0]
	}

	base := strings.Split(cleaned[0], string(os.PathSeparator))

	for _, p := range cleaned[1:] {
		curr := strings.Split(p, string(os.PathSeparator))
		limit := len(base)
		if len(curr) < limit {
			limit = len(curr)
		}
		cutAt := limit
		for i := 0; i < limit; i++ {
			if base[i] != curr[i] {
				cutAt = i
				break
			}
		}
		base = base[:cutAt]
	}

	result := strings.Join(base, string(os.PathSeparator))
	if result == "" {
		return string(os.PathSeparator)
	}
	return result
}

func addFileToZip(zw *zip.Writer, absPath, zipEntryPath, password string, buf []byte) error {
	zipEntryPath = filepath.ToSlash(zipEntryPath)

	file, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("datei '%s' nicht lesbar: %w", absPath, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat '%s' fehlgeschlagen: %w", absPath, err)
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("header für '%s' fehlgeschlagen: %w", absPath, err)
	}

	header.Name = zipEntryPath
	header.Method = zip.Deflate

	var fw io.Writer
	if password != "" {
		fw, err = zw.Encrypt(zipEntryPath, password)
	} else {
		fw, err = zw.CreateHeader(header)
	}

	if err != nil {
		return fmt.Errorf("zip-entry für '%s' konnte nicht erstellt werden: %w", zipEntryPath, err)
	}

	if _, err := io.CopyBuffer(fw, file, buf); err != nil {
		return fmt.Errorf("kopieren von '%s' fehlgeschlagen: %w", absPath, err)
	}

	return nil
}

func zipCreate(zipPath string, paths []string, password string, flat bool) (int, error) {
	if len(paths) == 0 {
		return 0, fmt.Errorf("keine quelldateien angegeben")
	}

	absZipPath, err := absPathStrict(zipPath)
	if err != nil {
		return 0, err
	}

	tmpPath := absZipPath + ".tmp"

	zf, err := os.Create(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("archiv konnte nicht erstellt werden: %w", err)
	}

	zipWriter := zip.NewWriter(zf)
	buf := make([]byte, 1024*1024)
	fileCount := 0

	baseFolder := ""
	if !flat {
		absPaths := make([]string, 0, len(paths))
		for _, p := range paths {
			if abs, err := absPathStrict(p); err == nil {
				absPaths = append(absPaths, abs)
			}
		}
		baseFolder = commonBasePath(absPaths)
	}

	for _, p := range paths {
		absP, err := absPathStrict(p)
		if err != nil {
			fmt.Printf("[ZIP-Warnung]: Überspringe '%s': %v\n", p, err)
			continue
		}

		info, err := os.Stat(absP)
		if err != nil {
			fmt.Printf("[ZIP-Warnung]: Überspringe '%s': %v\n", p, err)
			continue
		}

		if info.IsDir() {
			filepath.WalkDir(absP, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					fmt.Printf("[ZIP-Warnung]: Überspringe '%s': %v\n", path, walkErr)
					return nil
				}
				if d.IsDir() {
					return nil
				}

				entryPath := filepath.Base(path)
				if !flat {
					if rel, err := filepath.Rel(baseFolder, path); err == nil {
						entryPath = rel
					}
				}

				if err := addFileToZip(zipWriter, path, entryPath, password, buf); err != nil {
					fmt.Printf("[ZIP-Warnung]: Überspringe '%s': %v\n", path, err)
				} else {
					fileCount++
				}
				return nil
			})
		} else {
			entryPath := filepath.Base(absP)
			if !flat {
				if rel, err := filepath.Rel(baseFolder, absP); err == nil {
					entryPath = rel
				}
			}

			if err := addFileToZip(zipWriter, absP, entryPath, password, buf); err != nil {
				fmt.Printf("[ZIP-Warnung]: Überspringe '%s': %v\n", absP, err)
			} else {
				fileCount++
			}
		}
	}

	zipWriter.Close()
	zf.Close()

	if fileCount == 0 {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("archiv leer: keine validen dateien gefunden")
	}

	if err := os.Rename(tmpPath, absZipPath); err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("umbenennen fehlgeschlagen: %w", err)
	}

	return fileCount, nil
}

func zipExtract(zipPath, dest, password string) error {
	absZip, err := absPathStrict(zipPath)
	if err != nil {
		return err
	}

	absDest, err := absPathStrict(dest)
	if err != nil {
		return err
	}

	r, err := zip.OpenReader(absZip)
	if err != nil {
		return fmt.Errorf("archiv '%s' konnte nicht geöffnet werden: %w", absZip, err)
	}
	defer r.Close()

	buf := make([]byte, 1024*1024)
	destPrefix := filepath.Clean(absDest) + string(os.PathSeparator)

	for _, f := range r.File {
		if password != "" {
			f.SetPassword(password)
		}

		entryName := decodeZipName(f)
		targetPath := filepath.Join(absDest, entryName)

		if !strings.HasPrefix(filepath.Clean(targetPath)+string(os.PathSeparator), destPrefix) {
			return fmt.Errorf("zip-slip erkannt: '%s' liegt außerhalb des zielordners", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, f.Mode()); err != nil {
				return fmt.Errorf("verzeichnis '%s' konnte nicht erstellt werden: %w", targetPath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("elternverzeichnis für '%s' konnte nicht erstellt werden: %w", targetPath, err)
		}

		outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf("datei '%s' konnte nicht erstellt werden: %w", targetPath, err)
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return fmt.Errorf("eintrag '%s' konnte nicht geöffnet werden: %w", f.Name, err)
		}

		_, copyErr := io.CopyBuffer(outFile, rc, buf)
		outFile.Close()
		rc.Close()

		if copyErr != nil {
			return fmt.Errorf("entpacken von '%s' fehlgeschlagen: %w", f.Name, copyErr)
		}
	}

	return nil
}

// ------------------------------------------------------------

func main() {}
