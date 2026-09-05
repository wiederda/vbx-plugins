package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
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

func stringResult(value string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "str",
		Str:  value,
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

// getFilePaths unterstützt:
//
// tar.Create("test.tar", "a.txt", "b.txt")
//
// und:
//
// files = ["a.txt", "b.txt"]
// tar.Create("test.tar", files)

func getFilePaths(args []jsonValue) []string {
	var paths []string

	for _, arg := range args {

		switch arg.Type {

		case "str":
			if strings.TrimSpace(arg.Str) != "" {
				paths = append(paths, arg.Str)
			}

		case "arr":
			for _, item := range arg.Arr {
				if item.Type == "str" && strings.TrimSpace(item.Str) != "" {
					paths = append(paths, item.Str)
				}
			}
		}
	}

	return paths
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {

	entries := []funcDesc{

		{
			Namespace:   "tar",
			Name:        "Create",
			Params:      "tarPath, files...",
			Description: "Erstellt ein TAR-Archiv.",
		},

		{
			Namespace:   "tar",
			Name:        "CreateFlat",
			Params:      "tarPath, files...",
			Description: "Erstellt ein TAR-Archiv ohne Ordnerstruktur.",
		},

		{
			Namespace:   "tar",
			Name:        "Extract",
			Params:      "archive, dest",
			Description: "Entpackt ein TAR-Archiv in ein Zielverzeichnis.",
		},

		{
			Namespace:   "tar",
			Name:        "List",
			Params:      "path",
			Description: "Gibt alle Einträge eines TAR-Archivs zurück.",
		},

		{
			Namespace:   "tar",
			Name:        "Exists",
			Params:      "tarPath, file",
			Description: "Prüft, ob eine Datei im TAR-Archiv existiert.",
		},

		{
			Namespace:   "tar",
			Name:        "Add",
			Params:      "tarPath, files...",
			Description: "Fügt Dateien zu einem bestehenden TAR-Archiv hinzu.",
		},

		{
			Namespace:   "tar",
			Name:        "ToGz",
			Params:      "tarPath, [deleteSource]",
			Description: "Komprimiert ein TAR-Archiv zu .tar.gz.",
		},

		{
			Namespace:   "tar",
			Name:        "GzCreate",
			Params:      "output, files...",
			Description: "Erstellt ein komprimiertes TAR.GZ-Archiv.",
		},

		{
			Namespace:   "tar",
			Name:        "GzCreateFlat",
			Params:      "output, files...",
			Description: "Erstellt ein TAR.GZ-Archiv ohne Ordnerstruktur.",
		},

		{
			Namespace:   "tar",
			Name:        "GzExtract",
			Params:      "archive, dest",
			Description: "Entpackt ein TAR.GZ-Archiv.",
		},

		{
			Namespace:   "tar",
			Name:        "GzList",
			Params:      "path",
			Description: "Gibt alle Einträge eines TAR.GZ-Archivs zurück.",
		},

		{
			Namespace:   "tar",
			Name:        "GzExists",
			Params:      "tarPath, file",
			Description: "Prüft, ob eine Datei in einem TAR.GZ-Archiv existiert.",
		},

		{
			Namespace:   "tar",
			Name:        "GzIsValid",
			Params:      "path",
			Description: "Prüft, ob eine Datei ein gültiges TAR.GZ-Archiv ist.",
		},
	}

	data, _ := json.Marshal(entries)

	return packBytes(data)
}

// ------------------------------------------------------------
// vbx_call
// ------------------------------------------------------------

//go:wasmexport vbx_call
func vbxCall(namePtr, nameLen, argsPtr, argsLen uint32) uint64 {

	name := string(readBytes(namePtr, nameLen))
	argsJSON := readBytes(argsPtr, argsLen)

	var args []jsonValue

	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return packBytes(
			errorResult("ungültige Argumente: " + err.Error()),
		)
	}

	switch name {

	case "Create":
		return packBytes(handleCreate(args))

	case "CreateFlat":
		return packBytes(handleCreateFlat(args))

	case "Extract":
		return packBytes(handleExtract(args))

	case "List":
		return packBytes(handleList(args))

	case "Exists":
		return packBytes(handleExists(args))

	case "Add":
		return packBytes(handleAdd(args))

	case "ToGz":
		return packBytes(handleToGz(args))

	case "GzCreate":
		return packBytes(handleGzCreate(args))

	case "GzCreateFlat":
		return packBytes(handleGzCreateFlat(args))

	case "GzExtract":
		return packBytes(handleGzExtract(args))

	case "GzList":
		return packBytes(handleGzList(args))

	case "GzExists":
		return packBytes(handleGzExists(args))

	case "GzIsValid":
		return packBytes(handleGzIsValid(args))

	default:
		return packBytes(
			errorResult("unbekannte Funktion: " + name),
		)
	}
}

// ------------------------------------------------------------
// Handler
// ------------------------------------------------------------

func handleCreate(args []jsonValue) []byte {

	if len(args) < 2 || args[0].Type != "str" {
		return errorResult(
			"Create erwartet tarPath und mindestens eine Datei",
		)
	}

	output := args[0].Str
	paths := getFilePaths(args[1:])

	if len(paths) == 0 {
		return boolResult(false)
	}

	ok, err := createTar(output, paths, false, false)

	if err != nil {
		return errorResult(err.Error())
	}

	return boolResult(ok)
}

func handleCreateFlat(args []jsonValue) []byte {

	if len(args) < 2 || args[0].Type != "str" {
		return errorResult(
			"CreateFlat erwartet tarPath und mindestens eine Datei",
		)
	}

	output := args[0].Str
	paths := getFilePaths(args[1:])

	if len(paths) == 0 {
		return boolResult(false)
	}

	ok, err := createTar(output, paths, true, false)

	if err != nil {
		return errorResult(err.Error())
	}

	return boolResult(ok)
}

func handleExtract(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult(
			"Extract erwartet archive und dest",
		)
	}

	if args[0].Type != "str" || args[1].Type != "str" {
		return errorResult(
			"Extract erwartet zwei String-Argumente",
		)
	}

	if err := extractArchive(
		args[0].Str,
		args[1].Str,
		false,
	); err != nil {
		return errorResult(err.Error())
	}

	return boolResult(true)
}

func handleList(args []jsonValue) []byte {

	if len(args) < 1 || args[0].Type != "str" {
		return errorResult(
			"List erwartet einen Archivpfad",
		)
	}

	entries, err := listArchive(
		args[0].Str,
		false,
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return arrayResult(entries)
}

func handleExists(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult(
			"Exists erwartet tarPath und file",
		)
	}

	if args[0].Type != "str" || args[1].Type != "str" {
		return errorResult(
			"Exists erwartet zwei String-Argumente",
		)
	}

	exists, err := existsInTar(
		args[0].Str,
		args[1].Str,
		false,
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return boolResult(exists)
}

func handleAdd(args []jsonValue) []byte {

	if len(args) < 2 || args[0].Type != "str" {
		return errorResult(
			"Add erwartet tarPath und mindestens eine Datei",
		)
	}

	paths := getFilePaths(args[1:])

	if len(paths) == 0 {
		return boolResult(false)
	}

	ok, err := addToTar(
		args[0].Str,
		paths,
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return boolResult(ok)
}

func handleToGz(args []jsonValue) []byte {

	if len(args) < 1 || args[0].Type != "str" {
		return errorResult(
			"ToGz erwartet tarPath",
		)
	}

	deleteSource := false

	if len(args) > 1 && args[1].Type == "bool" {
		deleteSource = args[1].Bool
	}

	path, err := convertTarToGz(
		args[0].Str,
		deleteSource,
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return stringResult(path)
}

func handleGzCreate(args []jsonValue) []byte {

	if len(args) < 2 || args[0].Type != "str" {
		return errorResult(
			"GzCreate erwartet output und mindestens eine Datei",
		)
	}

	paths := getFilePaths(args[1:])

	if len(paths) == 0 {
		return boolResult(false)
	}

	ok, err := createTar(
		args[0].Str,
		paths,
		false,
		true,
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return boolResult(ok)
}

func handleGzCreateFlat(args []jsonValue) []byte {

	if len(args) < 2 || args[0].Type != "str" {
		return errorResult(
			"GzCreateFlat erwartet output und mindestens eine Datei",
		)
	}

	paths := getFilePaths(args[1:])

	if len(paths) == 0 {
		return boolResult(false)
	}

	ok, err := createTar(
		args[0].Str,
		paths,
		true,
		true,
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return boolResult(ok)
}

func handleGzExtract(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult(
			"GzExtract erwartet archive und dest",
		)
	}

	if args[0].Type != "str" || args[1].Type != "str" {
		return errorResult(
			"GzExtract erwartet zwei String-Argumente",
		)
	}

	if err := extractArchive(
		args[0].Str,
		args[1].Str,
		true,
	); err != nil {
		return errorResult(err.Error())
	}

	return boolResult(true)
}

func handleGzList(args []jsonValue) []byte {

	if len(args) < 1 || args[0].Type != "str" {
		return errorResult(
			"GzList erwartet einen Archivpfad",
		)
	}

	entries, err := listArchive(
		args[0].Str,
		true,
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return arrayResult(entries)
}

func handleGzExists(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult(
			"GzExists erwartet tarPath und file",
		)
	}

	if args[0].Type != "str" || args[1].Type != "str" {
		return errorResult(
			"GzExists erwartet zwei String-Argumente",
		)
	}

	exists, err := existsInTar(
		args[0].Str,
		args[1].Str,
		true,
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return boolResult(exists)
}

func handleGzIsValid(args []jsonValue) []byte {

	if len(args) < 1 || args[0].Type != "str" {
		return errorResult(
			"GzIsValid erwartet einen Archivpfad",
		)
	}

	ok := isValidGzTar(args[0].Str)

	return boolResult(ok)
}

// ------------------------------------------------------------
// TAR erstellen
// ------------------------------------------------------------

func createTar(
	output string,
	paths []string,
	flat bool,
	gzipEnabled bool,
) (bool, error) {

	if len(paths) == 0 {
		return false, nil
	}

	outFile, err := os.Create(output)
	if err != nil {
		return false, err
	}

	success := false
	fileCount := 0

	defer func() {
		outFile.Close()

		if !success || fileCount == 0 {
			os.Remove(output)
		}
	}()

	var writer io.Writer = outFile

	var gzWriter *gzip.Writer

	if gzipEnabled {

		gzWriter, err = gzip.NewWriterLevel(
			outFile,
			gzip.DefaultCompression,
		)

		if err != nil {
			return false, err
		}

		writer = gzWriter
	}

	tw := tar.NewWriter(writer)

	base := ""

	if !flat {
		base = commonBasePath(paths)
	}

	for _, path := range paths {

		if strings.TrimSpace(path) == "" {
			continue
		}

		if err := addPathToTar(
			tw,
			path,
			base,
			flat,
			&fileCount,
		); err != nil {

			tw.Close()

			if gzWriter != nil {
				gzWriter.Close()
			}

			return false, err
		}
	}

	if err := tw.Close(); err != nil {
		return false, err
	}

	if gzWriter != nil {
		if err := gzWriter.Close(); err != nil {
			return false, err
		}
	}

	success = fileCount > 0

	return success, nil
}

// ------------------------------------------------------------
// Datei / Verzeichnis zu TAR hinzufügen
// ------------------------------------------------------------

func addPathToTar(
	tw *tar.Writer,
	path string,
	base string,
	flat bool,
	count *int,
) error {

	return filepath.WalkDir(
		path,
		func(
			currPath string,
			d os.DirEntry,
			err error,
		) error {

			if err != nil {
				// Einzelne Fehler überspringen
				return nil
			}

			if d.IsDir() && flat {
				return nil
			}

			info, err := d.Info()

			if err != nil {
				return nil
			}

			var name string

			if flat {

				name = d.Name()

			} else {

				rel, err := filepath.Rel(
					base,
					currPath,
				)

				if err != nil {
					name = d.Name()
				} else {
					name = rel
				}
			}

			header, err := tar.FileInfoHeader(
				info,
				"",
			)

			if err != nil {
				return nil
			}

			header.Name = filepath.ToSlash(name)

			if d.IsDir() {

				if !strings.HasSuffix(
					header.Name,
					"/",
				) {
					header.Name += "/"
				}
			}

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			file, err := os.Open(currPath)

			if err != nil {
				return nil
			}

			defer file.Close()

			if _, err := io.Copy(
				tw,
				file,
			); err != nil {
				return err
			}

			(*count)++

			return nil
		},
	)
}

// ------------------------------------------------------------
// TAR entpacken
// ------------------------------------------------------------

func isSafePath(base, target string) bool {

	cleanBase := filepath.Clean(base)
	cleanTarget := filepath.Clean(target)

	if !strings.HasSuffix(
		cleanBase,
		string(os.PathSeparator),
	) {
		cleanBase += string(os.PathSeparator)
	}

	targetWithSeparator := cleanTarget

	if !strings.HasSuffix(
		targetWithSeparator,
		string(os.PathSeparator),
	) {
		targetWithSeparator += string(os.PathSeparator)
	}

	return strings.HasPrefix(
		targetWithSeparator,
		cleanBase,
	)
}

func extractArchive(
	archivePath string,
	dest string,
	gzipEnabled bool,
) error {

	cleanDest, err := filepath.Abs(dest)

	if err != nil {
		return err
	}

	if err := os.MkdirAll(
		cleanDest,
		0755,
	); err != nil {
		return err
	}

	srcPath, err := filepath.Abs(archivePath)

	if err != nil {
		return err
	}

	file, err := os.Open(srcPath)

	if err != nil {
		return err
	}

	defer file.Close()

	var reader io.Reader = file

	var gzReader *gzip.Reader

	if gzipEnabled {

		gzReader, err = gzip.NewReader(file)

		if err != nil {
			return err
		}

		defer gzReader.Close()

		reader = gzReader
	}

	tr := tar.NewReader(reader)

	buffer := make(
		[]byte,
		1024*1024,
	)

	for {

		header, err := tr.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		target := filepath.Join(
			cleanDest,
			header.Name,
		)

		// Schutz gegen ../ Path Traversal
		if !isSafePath(
			cleanDest,
			target,
		) {
			continue
		}

		switch header.Typeflag {

		case tar.TypeDir:

			if err := os.MkdirAll(
				target,
				0755,
			); err != nil {
				return err
			}

		case tar.TypeReg:

			if err := os.MkdirAll(
				filepath.Dir(target),
				0755,
			); err != nil {
				return err
			}

			outFile, err := os.OpenFile(
				target,
				os.O_WRONLY|
					os.O_CREATE|
					os.O_TRUNC,
				header.FileInfo().Mode(),
			)

			if err != nil {
				return err
			}

			_, copyErr := io.CopyBuffer(
				outFile,
				tr,
				buffer,
			)

			closeErr := outFile.Close()

			if copyErr != nil {
				return copyErr
			}

			if closeErr != nil {
				return closeErr
			}
		}
	}

	return nil
}

// ------------------------------------------------------------
// Archiv auflisten
// ------------------------------------------------------------

func listArchive(
	archivePath string,
	gzipEnabled bool,
) ([]jsonValue, error) {

	srcPath, err := filepath.Abs(archivePath)

	if err != nil {
		return nil, err
	}

	file, err := os.Open(srcPath)

	if err != nil {
		return nil, err
	}

	defer file.Close()

	var reader io.Reader = file

	var gzReader *gzip.Reader

	if gzipEnabled {

		gzReader, err = gzip.NewReader(file)

		if err != nil {
			return nil, err
		}

		defer gzReader.Close()

		reader = gzReader
	}

	tr := tar.NewReader(reader)

	var entries []jsonValue

	for {

		header, err := tr.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		entry := jsonValue{
			Type: "map",

			Map: map[string]jsonValue{

				"Name": {
					Type: "str",
					Str:  header.Name,
				},

				"Size": {
					Type: "num",
					Num:  float64(header.Size),
				},

				"IsDir": {
					Type: "bool",
					Bool: header.Typeflag == tar.TypeDir,
				},

				"ModTime": {
					Type: "str",
					Str: header.ModTime.Format(
						"2006-01-02 15:04:05",
					),
				},
			},
		}

		entries = append(
			entries,
			entry,
		)
	}

	return entries, nil
}

// ------------------------------------------------------------
// Datei im Archiv suchen
// ------------------------------------------------------------

func existsInTar(
	archivePath string,
	fileName string,
	gzipEnabled bool,
) (bool, error) {

	file, err := os.Open(archivePath)

	if err != nil {
		return false, err
	}

	defer file.Close()

	var reader io.Reader = file

	var gzReader *gzip.Reader

	if gzipEnabled {

		gzReader, err = gzip.NewReader(file)

		if err != nil {
			return false, err
		}

		defer gzReader.Close()

		reader = gzReader
	}

	tr := tar.NewReader(reader)

	for {

		header, err := tr.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return false, err
		}

		if header.Name == fileName {
			return true, nil
		}
	}

	return false, nil
}

// ------------------------------------------------------------
// Dateien zu bestehendem TAR hinzufügen
// ------------------------------------------------------------

func addToTar(
	tarPath string,
	paths []string,
) (bool, error) {

	if _, err := os.Stat(tarPath); os.IsNotExist(err) {

		return createTar(
			tarPath,
			paths,
			false,
			false,
		)
	}

	f, err := os.OpenFile(
		tarPath,
		os.O_RDWR,
		0644,
	)

	if err != nil {
		return false, err
	}

	defer f.Close()

	offset, err := findTarAppendOffset(f)

	if err != nil {
		return false, err
	}

	if _, err = f.Seek(
		offset,
		io.SeekStart,
	); err != nil {
		return false, err
	}

	tw := tar.NewWriter(f)

	base := commonBasePath(paths)

	fileCount := 0

	for _, path := range paths {

		if err := addPathToTar(
			tw,
			path,
			base,
			false,
			&fileCount,
		); err != nil {

			tw.Close()

			return false, err
		}
	}

	if err := tw.Close(); err != nil {
		return false, err
	}

	return fileCount > 0, nil
}

// ------------------------------------------------------------
// TAR zu TAR.GZ konvertieren
// ------------------------------------------------------------

func convertTarToGz(
	tarPath string,
	deleteSource bool,
) (string, error) {

	srcFile, err := os.Open(tarPath)

	if err != nil {
		return "", fmt.Errorf(
			"Quelle konnte nicht geöffnet werden: %w",
			err,
		)
	}

	defer srcFile.Close()

	fileInfo, err := srcFile.Stat()

	if err != nil {
		return "", fmt.Errorf(
			"Stat fehlgeschlagen: %w",
			err,
		)
	}

	gzPath := tarPath + ".gz"

	dstFile, err := os.Create(gzPath)

	if err != nil {
		return "", fmt.Errorf(
			"Ziel konnte nicht erstellt werden: %w",
			err,
		)
	}

	success := false

	defer func() {

		dstFile.Close()

		if !success {
			os.Remove(gzPath)
		}
	}()

	gzWriter, err := gzip.NewWriterLevel(
		dstFile,
		gzip.BestCompression,
	)

	if err != nil {
		return "", err
	}

	gzWriter.Name = filepath.Base(tarPath)
	gzWriter.ModTime = fileInfo.ModTime()

	if _, err = io.Copy(
		gzWriter,
		srcFile,
	); err != nil {

		gzWriter.Close()

		return "", fmt.Errorf(
			"Kompression fehlgeschlagen: %w",
			err,
		)
	}

	if err := gzWriter.Close(); err != nil {

		return "", fmt.Errorf(
			"Gzip-Abschluss fehlgeschlagen: %w",
			err,
		)
	}

	success = true

	if deleteSource {

		if err := os.Remove(tarPath); err != nil {
			return "", err
		}
	}

	return gzPath, nil
}

// ------------------------------------------------------------
// TAR Append Offset finden
// ------------------------------------------------------------

func findTarAppendOffset(
	f *os.File,
) (int64, error) {

	if _, err := f.Seek(
		0,
		io.SeekStart,
	); err != nil {
		return 0, err
	}

	tr := tar.NewReader(f)

	for {

		_, err := tr.Next()

		if err == io.EOF {

			offset, err := f.Seek(
				0,
				io.SeekCurrent,
			)

			if err != nil {
				return 0, err
			}

			if offset < 1024 {
				return 0, fmt.Errorf(
					"TAR zu klein oder leer",
				)
			}

			return offset - 1024, nil
		}

		if err != nil {

			return 0, fmt.Errorf(
				"korruptes TAR-Archiv: %w",
				err,
			)
		}
	}
}

// ------------------------------------------------------------
// TAR.GZ Validierung
// ------------------------------------------------------------

func isValidGzTar(path string) bool {

	file, err := os.Open(path)

	if err != nil {
		return false
	}

	defer file.Close()

	gzReader, err := gzip.NewReader(file)

	if err != nil {
		return false
	}

	defer gzReader.Close()

	tr := tar.NewReader(gzReader)

	_, err = tr.Next()

	if err != nil {
		return false
	}

	return true
}

// ------------------------------------------------------------
// Gemeinsamen Basispfad bestimmen
// ------------------------------------------------------------

func commonBasePath(paths []string) string {

	if len(paths) == 0 {
		return ""
	}

	base, err := filepath.Abs(paths[0])

	if err != nil {
		return "."
	}

	info, err := os.Stat(base)

	if err == nil && !info.IsDir() {
		base = filepath.Dir(base)
	}

	for _, path := range paths[1:] {

		absPath, err := filepath.Abs(path)

		if err != nil {
			continue
		}

		info, err := os.Stat(absPath)

		if err == nil && !info.IsDir() {
			absPath = filepath.Dir(absPath)
		}

		for {

			rel, err := filepath.Rel(
				base,
				absPath,
			)

			if err != nil {
				break
			}

			if rel != ".." &&
				!strings.HasPrefix(
					rel,
					".."+string(os.PathSeparator),
				) {
				break
			}

			parent := filepath.Dir(base)

			if parent == base {
				break
			}

			base = parent
		}
	}

	return base
}

// ------------------------------------------------------------

func main() {}
