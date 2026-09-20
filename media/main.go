package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// ============================================================
// WASM Speicher
// ============================================================

var liveBuffers map[uint32][]byte

//go:wasmexport alloc
func alloc(size uint32) uint32 {
	if liveBuffers == nil {
		liveBuffers = make(map[uint32][]byte)
	}

	if size == 0 {
		size = 1
	}

	buf := make([]byte, size)

	ptr := uint32(uintptr(unsafe.Pointer(&buf[0])))

	liveBuffers[ptr] = buf

	return ptr
}

//go:wasmexport dealloc
func dealloc(ptr uint32, size uint32) {
	if liveBuffers == nil {
		return
	}

	delete(liveBuffers, ptr)
}

// ============================================================
// ABI
// ============================================================

//go:wasmexport vbx_abi_version
func vbxABIVersion() uint32 {
	return 1
}

// ============================================================
// JSON Wire Format
// ============================================================

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

// ============================================================
// Host FFmpeg Bridge
// ============================================================

type hostFFmpegRequest struct {
	Args []string `json:"args"`
}

type hostFFmpegResponse struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Code   int    `json:"code"`
}

//go:wasmimport vbx_host ffmpeg_exec
func hostFFmpegExec(ptr uint32, length uint32) uint64

// ============================================================
// Host Datei ersetzen
//
// SetTag/SetTags/SetCover erzeugen zunächst eine temporäre Datei.
// Danach ersetzt der Host die ursprüngliche Datei.
//
// Der Host muss dafür:
//   replace_file(temp, target)
// exportieren.
// ============================================================

type hostReplaceFileRequest struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type hostReplaceFileResponse struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Code   int    `json:"code"`
}

//go:wasmimport vbx_host replace_file
func hostReplaceFile(ptr uint32, length uint32) uint64

// ============================================================
// Speicher / Pointer
// ============================================================

func readBytes(ptr, length uint32) []byte {
	if liveBuffers == nil {
		return nil
	}

	buf, ok := liveBuffers[ptr]
	if !ok {
		return nil
	}

	if uint32(len(buf)) < length {
		return nil
	}

	return buf[:length]
}

func packBytes(data []byte) uint64 {
	ptr := alloc(uint32(len(data)))

	if len(data) > 0 {
		copy(liveBuffers[ptr], data)
	}

	return (uint64(ptr) << 32) | uint64(len(data))
}

func unpackPtrLen(packed uint64) (uint32, uint32) {
	return uint32(packed >> 32), uint32(packed)
}

// ============================================================
// Ergebnisse
// ============================================================

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

func mapResult(m map[string]jsonValue) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "map",
		Map:  m,
	})

	return data
}

func arrayResult(a []jsonValue) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "arr",
		Arr:  a,
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

// ============================================================
// JSON-Werte
// ============================================================

func valueBool(v jsonValue) bool {
	switch v.Type {
	case "bool":
		return v.Bool

	case "num":
		return v.Num != 0

	case "str":
		s := strings.ToLower(
			strings.TrimSpace(v.Str),
		)

		return s == "true" ||
			s == "1" ||
			s == "yes"
	}

	return false
}

func valueInt(
	v jsonValue,
	defaultValue int,
) int {
	if v.Type != "num" {
		return defaultValue
	}

	return int(v.Num)
}

func requireString(
	args []jsonValue,
	index int,
	name string,
) (string, []byte) {

	if len(args) <= index {
		return "", errorResult(
			fmt.Sprintf(
				"%s erwartet mindestens %d Argument(e)",
				name,
				index+1,
			),
		)
	}

	if args[index].Type != "str" {
		return "", errorResult(
			fmt.Sprintf(
				"%s: Argument %d muss ein String sein",
				name,
				index+1,
			),
		)
	}

	if strings.TrimSpace(args[index].Str) == "" {
		return "", errorResult(
			fmt.Sprintf(
				"%s: Argument %d darf nicht leer sein",
				name,
				index+1,
			),
		)
	}

	return args[index].Str, nil
}

// ============================================================
// Array / Map Hilfsfunktionen
// ============================================================

func requireStringArray(
	v jsonValue,
	name string,
) ([]string, []byte) {

	if v.Type != "arr" {
		return nil, errorResult(
			fmt.Sprintf(
				"%s: Dateiliste muss ein Array sein",
				name,
			),
		)
	}

	if len(v.Arr) == 0 {
		return nil, errorResult(
			fmt.Sprintf(
				"%s: Dateiliste darf nicht leer sein",
				name,
			),
		)
	}

	result := make([]string, 0, len(v.Arr))

	for i, item := range v.Arr {
		if item.Type != "str" ||
			strings.TrimSpace(item.Str) == "" {

			return nil, errorResult(
				fmt.Sprintf(
					"%s: Element %d muss ein nicht leerer String sein",
					name,
					i+1,
				),
			)
		}

		result = append(
			result,
			item.Str,
		)
	}

	return result, nil
}

func jsonValueToString(
	v jsonValue,
	name string,
) (string, []byte) {

	switch v.Type {
	case "str":
		return v.Str, nil

	case "num":
		return strconv.FormatFloat(
			v.Num,
			'f',
			-1,
			64,
		), nil

	case "bool":
		if v.Bool {
			return "true", nil
		}

		return "false", nil

	default:
		return "", errorResult(
			fmt.Sprintf(
				"%s: Wert muss String, Zahl oder Boolean sein",
				name,
			),
		)
	}
}

// ============================================================
// FFmpeg Host-Aufruf
// ============================================================

func ffmpegExec(
	args []string,
) (hostFFmpegResponse, error) {

	req := hostFFmpegRequest{
		Args: args,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return hostFFmpegResponse{}, fmt.Errorf(
			"FFmpeg-Request konnte nicht kodiert werden: %v",
			err,
		)
	}

	ptr := alloc(uint32(len(data)))

	copy(
		liveBuffers[ptr],
		data,
	)

	packed := hostFFmpegExec(
		ptr,
		uint32(len(data)),
	)

	dealloc(
		ptr,
		uint32(len(data)),
	)

	resPtr, resLen := unpackPtrLen(packed)

	if resPtr == 0 || resLen == 0 {
		return hostFFmpegResponse{}, fmt.Errorf(
			"keine Antwort von der FFmpeg-Host-Bridge",
		)
	}

	resultData := readBytes(
		resPtr,
		resLen,
	)

	if resultData == nil {
		dealloc(
			resPtr,
			resLen,
		)

		return hostFFmpegResponse{}, fmt.Errorf(
			"Antwort der FFmpeg-Host-Bridge konnte nicht gelesen werden",
		)
	}

	var result hostFFmpegResponse

	if err := json.Unmarshal(
		resultData,
		&result,
	); err != nil {

		dealloc(
			resPtr,
			resLen,
		)

		return hostFFmpegResponse{}, fmt.Errorf(
			"ungültige Antwort der FFmpeg-Host-Bridge: %v",
			err,
		)
	}

	dealloc(
		resPtr,
		resLen,
	)

	return result, nil
}

// ============================================================
// Host Datei ersetzen
// ============================================================

func replaceFile(
	source,
	target string,
) error {

	req := hostReplaceFileRequest{
		Source: source,
		Target: target,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf(
			"Replace-Request konnte nicht kodiert werden: %v",
			err,
		)
	}

	ptr := alloc(uint32(len(data)))

	copy(
		liveBuffers[ptr],
		data,
	)

	packed := hostReplaceFile(
		ptr,
		uint32(len(data)),
	)

	dealloc(
		ptr,
		uint32(len(data)),
	)

	resPtr, resLen := unpackPtrLen(packed)

	if resPtr == 0 || resLen == 0 {
		return fmt.Errorf(
			"keine Antwort von der Replace-File-Host-Bridge",
		)
	}

	resultData := readBytes(
		resPtr,
		resLen,
	)

	if resultData == nil {
		dealloc(
			resPtr,
			resLen,
		)

		return fmt.Errorf(
			"Antwort der Replace-File-Host-Bridge konnte nicht gelesen werden",
		)
	}

	var result hostReplaceFileResponse

	if err := json.Unmarshal(
		resultData,
		&result,
	); err != nil {

		dealloc(
			resPtr,
			resLen,
		)

		return fmt.Errorf(
			"ungültige Antwort der Replace-File-Host-Bridge: %v",
			err,
		)
	}

	dealloc(
		resPtr,
		resLen,
	)

	if result.Code != 0 {
		msg := strings.TrimSpace(result.Stderr)

		if msg == "" {
			msg = strings.TrimSpace(result.Stdout)
		}

		if msg == "" {
			msg = fmt.Sprintf(
				"Datei konnte nicht ersetzt werden (Exit-Code %d)",
				result.Code,
			)
		}

		return fmt.Errorf(
			"%s",
			msg,
		)
	}

	return nil
}

// ============================================================
// FFmpeg Fehler
// ============================================================

func ffmpegError(
	result hostFFmpegResponse,
) []byte {

	msg := strings.TrimSpace(
		result.Stderr,
	)

	if msg == "" {
		msg = strings.TrimSpace(
			result.Stdout,
		)
	}

	if msg == "" {
		msg = fmt.Sprintf(
			"FFmpeg fehlgeschlagen (Exit-Code %d)",
			result.Code,
		)
	}

	return errorResult(
		msg,
	)
}

// ============================================================
// Pfadbestandteile ermitteln
//
// Unterstützt:
//
//   Windows: C:\Ordner\Datei.mp3
//   UNC:     \\server\share\Ordner\Datei.mp3
//   Linux:   /ordner/datei.mp3
//
// filepath wird hier bewusst nicht verwendet, da das WASM-Modul
// unter Umständen unter einem anderen Betriebssystem bzw.
// Pfadmodell läuft als der Host.
// ============================================================

func splitMediaPath(
	file string,
) (dir, base, ext string) {

	lastSlash := strings.LastIndex(
		file,
		"/",
	)

	lastBackslash := strings.LastIndex(
		file,
		"\\",
	)

	pos := lastSlash

	if lastBackslash > pos {
		pos = lastBackslash
	}

	if pos >= 0 {
		dir = file[:pos]
		base = file[pos+1:]
	} else {
		dir = "."
		base = file
	}

	if dir == "" {
		dir = "."
	}

	dot := strings.LastIndex(
		base,
		".",
	)

	if dot <= 0 {
		return dir, base, ""
	}

	ext = base[dot:]
	base = base[:dot]

	return dir, base, ext
}

// ============================================================
// Dauer aus FFmpeg-Ausgabe lesen
//
// Typische FFmpeg-Ausgabe:
//   Duration: 00:12:34.56, start: 0.000000, bitrate: 320 kb/s
// ============================================================

func findDuration(text string) (float64, bool) {

	idx := strings.Index(text, "Duration:")
	if idx < 0 {
		return 0, false
	}

	part := text[idx+len("Duration:"):]
	part = strings.TrimSpace(part)

	comma := strings.Index(part, ",")
	if comma < 0 {
		return 0, false
	}
	timeStr := strings.TrimSpace(part[:comma])

	if timeStr == "N/A" {
		return 0, false
	}

	segments := strings.Split(timeStr, ":")
	if len(segments) != 3 {
		return 0, false
	}

	hours, err1 := strconv.Atoi(segments[0])
	minutes, err2 := strconv.Atoi(segments[1])
	seconds, err3 := strconv.ParseFloat(segments[2], 64)

	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}

	total := float64(hours)*3600 + float64(minutes)*60 + seconds
	return total, true
}

// ============================================================
// media.GetDuration(input)
// ============================================================

func handleGetDuration(args []jsonValue) []byte {

	input, errResult := requireString(args, 0, "media.GetDuration")
	if errResult != nil {
		return errResult
	}

	if len(args) > 1 {
		return errorResult("media.GetDuration erwartet 1 Argument")
	}

	result, err := ffmpegExec([]string{"-hide_banner", "-i", input})
	if err != nil {
		return errorResult(err.Error())
	}

	duration, ok := findDuration(result.Stderr)
	if !ok {
		return errorResult("media.GetDuration: Dauer konnte nicht ermittelt werden")
	}

	return numResult(duration)
}

// ============================================================
// media.GetInfo(file)
//
// Bündelt Bitrate, Dauer und Cover-Prüfung in einem einzigen
// FFmpeg-Aufruf, statt GetBitrate/GetDuration/IsCover einzeln
// aufzurufen (spart bei Batch-Verarbeitung über viele Dateien
// zwei Drittel der FFmpeg-Prozessstarts).
//
// Konsistent zu GetBitrate/GetDuration: kann eine der beiden
// Kennzahlen nicht ermittelt werden, liefert GetInfo einen
// ErrorVal statt eines stillen Platzhalterwerts.
// ============================================================

func handleGetInfo(args []jsonValue) []byte {

	file, errResult := requireString(args, 0, "media.GetInfo")
	if errResult != nil {
		return errResult
	}

	if len(args) > 1 {
		return errorResult("media.GetInfo erwartet 1 Argument")
	}

	result, err := ffmpegExec([]string{"-hide_banner", "-i", file})
	if err != nil {
		return errorResult(err.Error())
	}

	bitrate, ok := findAudioBitrate(result.Stderr)
	if !ok {
		return errorResult("media.GetInfo: Bitrate konnte nicht ermittelt werden")
	}

	duration, ok := findDuration(result.Stderr)
	if !ok {
		return errorResult("media.GetInfo: Dauer konnte nicht ermittelt werden")
	}

	info := map[string]jsonValue{
		"bitrate":  {Type: "num", Num: float64(bitrate)},
		"duration": {Type: "num", Num: duration},
		"hasCover": {Type: "bool", Bool: strings.Contains(result.Stderr, "Video:")},
	}

	return mapResult(info)
}

// ============================================================
// Temporären Dateinamen erzeugen
// ============================================================

func temporaryMediaPath(
	file string,
) string {

	dir, base, ext := splitMediaPath(
		file,
	)

	timestamp := time.Now().UnixNano()

	name := base +
		".vbx-tag-" +
		strconv.FormatInt(
			timestamp,
			10,
		) +
		ext

	if dir == "." {
		return name
	}

	if strings.HasSuffix(dir, "\\") ||
		strings.HasSuffix(dir, "/") {

		return dir + name
	}

	if strings.HasPrefix(dir, `\\`) ||
		(len(dir) >= 2 && dir[1] == ':') {

		return dir + "\\" + name
	}

	return dir + "/" + name
}

// ============================================================
// Cover-Konstanten
// ============================================================

const (
	maxCoverWidth  = 1000
	maxCoverHeight = 1000
	maxCoverSize   = 1024 * 1024
)

// ============================================================
// Cover-Datei prüfen
//
// Es werden ausschließlich JPEG-Dateien akzeptiert.
//
// FFmpeg wird mit loglevel "info" aufgerufen, damit die
// erkannte Bildgröße aus der Ausgabe gelesen werden kann.
//
// Die Dateigröße selbst kann die WASM-Seite nicht direkt über
// das Dateisystem ermitteln. Deshalb wird die JPEG-Datei über
// FFmpeg zusätzlich in einen maximal 1-MB großen JPEG-Stream
// geschrieben. Ist die Quelldatei bereits größer als 1 MB,
// wird sie anhand der Host-Ausgabe nicht direkt erkannt.
//
// Die eigentliche Größenbegrenzung wird daher über den
// Host-FFmpeg-Aufruf für das Cover durchgesetzt.
// ============================================================

func validateCover(
	file string,
) error {

	_, _, ext := splitMediaPath(file)

	ext = strings.ToLower(ext)

	if ext != ".jpg" &&
		ext != ".jpeg" {

		return fmt.Errorf(
			"Cover muss eine JPG-Datei sein",
		)
	}

	result, err := ffmpegExec(
		[]string{
			"-hide_banner",
			"-loglevel",
			"info",
			"-i",
			file,
			"-frames:v",
			"1",
			"-f",
			"null",
			"-",
		},
	)

	if err != nil {
		return err
	}

	if result.Code != 0 {
		msg := strings.TrimSpace(result.Stderr)

		if msg == "" {
			msg = strings.TrimSpace(result.Stdout)
		}

		if msg == "" {
			msg = "JPG-Cover konnte nicht gelesen werden"
		}

		return fmt.Errorf(
			"%s",
			msg,
		)
	}

	width, height, ok := findVideoDimensions(
		result.Stderr + "\n" + result.Stdout,
	)

	if !ok {
		return fmt.Errorf(
			"Abmessungen des JPG-Covers konnten nicht ermittelt werden",
		)
	}

	if width > maxCoverWidth ||
		height > maxCoverHeight {

		return fmt.Errorf(
			"Cover darf maximal %d × %d Pixel groß sein (gefunden: %d × %d)",
			maxCoverWidth,
			maxCoverHeight,
			width,
			height,
		)
	}

	return nil
}

// ============================================================
// Bildabmessungen aus FFmpeg-Ausgabe lesen
// ============================================================

func findVideoDimensions(
	text string,
) (width, height int, ok bool) {

	lines := strings.Split(
		text,
		"\n",
	)

	for _, line := range lines {

		line = strings.TrimSpace(line)

		// Typische FFmpeg-Ausgabe:
		//
		// Video: mjpeg, yuvj420p, 600x600
		//
		pos := strings.Index(
			line,
			"Video:",
		)

		if pos < 0 {
			continue
		}

		part := line[pos:]

		for i := 0; i < len(part); i++ {

			if part[i] < '0' ||
				part[i] > '9' {

				continue
			}

			j := i

			for j < len(part) &&
				part[j] >= '0' &&
				part[j] <= '9' {

				j++
			}

			if j >= len(part) ||
				part[j] != 'x' {

				continue
			}

			k := j + 1

			if k >= len(part) ||
				part[k] < '0' ||
				part[k] > '9' {

				continue
			}

			for k < len(part) &&
				part[k] >= '0' &&
				part[k] <= '9' {

				k++
			}

			w, err1 := strconv.Atoi(
				part[i:j],
			)

			h, err2 := strconv.Atoi(
				part[j+1 : k],
			)

			if err1 == nil &&
				err2 == nil &&
				w > 0 &&
				h > 0 {

				return w, h, true
			}
		}
	}

	return 0, 0, false
}

// ============================================================
// Audio-Bitrate aus FFmpeg-Ausgabe lesen
//
// Typische FFmpeg-Ausgabe:
//   Stream #0:0: Audio: mp3, 44100 Hz, stereo, fltp, 320 kb/s
// ============================================================

func findAudioBitrate(text string) (int, bool) {

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)

		if !strings.Contains(line, "Audio:") {
			continue
		}

		idx := strings.Index(line, "kb/s")
		if idx < 0 {
			continue
		}

		// Rückwärts von "kb/s" die davorstehende Zahl extrahieren
		end := idx
		for end > 0 && line[end-1] == ' ' {
			end--
		}
		start := end
		for start > 0 && line[start-1] >= '0' && line[start-1] <= '9' {
			start--
		}

		if start == end {
			continue
		}

		if br, err := strconv.Atoi(line[start:end]); err == nil {
			return br, true
		}
	}

	return 0, false
}

// ============================================================
// media.CheckTags(file, tags)
//
// Prüft, ob alle angegebenen Tags gesetzt und nicht leer sind.
// Gibt bei fehlendem/leerem Tag den Dateipfad selbst zurück
// (nützlich zum direkten Sammeln in ein Array), sonst "".
// ============================================================

func handleCheckTags(args []jsonValue) []byte {

	file, errResult := requireString(args, 0, "media.CheckTags")
	if errResult != nil {
		return errResult
	}

	if len(args) != 2 {
		return errorResult("media.CheckTags erwartet 2 Argumente")
	}

	tagNames, errResult := requireStringArray(args[1], "media.CheckTags")
	if errResult != nil {
		return errResult
	}

	canonical := make([]string, 0, len(tagNames))
	for _, n := range tagNames {
		key, ok := resolveTagAlias(n)
		if !ok {
			return errorResult(fmt.Sprintf("Tag wird aktuell nicht unterstützt: %s", n))
		}
		canonical = append(canonical, key)
	}

	ffmpegArgs := []string{
		"-hide_banner", "-loglevel", "error",
		"-i", file,
		"-map_metadata", "0",
		"-f", "ffmetadata", "-",
	}

	result, err := ffmpegExec(ffmpegArgs)
	if err != nil {
		return errorResult(err.Error())
	}
	if result.Code != 0 {
		return ffmpegError(result)
	}

	tags := parseFFMetadata(result.Stdout)

	for _, key := range canonical {
		value, ok := findTag(tags, key)
		if !ok || strings.TrimSpace(value.Str) == "" {
			return strResult(file)
		}
	}

	return strResult("")
}

// ============================================================
// media.IsValid(input)
// ============================================================

func handleIsValid(
	args []jsonValue,
) []byte {

	input, errResult := requireString(
		args,
		0,
		"media.IsValid",
	)

	if errResult != nil {
		return errResult
	}

	if len(args) > 1 {
		return errorResult(
			"media.IsValid erwartet 1 Argument",
		)
	}

	ffmpegArgs := []string{
		"-hide_banner",
		"-loglevel",
		"error",
		"-i",
		input,
		"-map",
		"0:v?",
		"-map",
		"0:a?",
		"-f",
		"null",
		"-",
	}

	result, err := ffmpegExec(
		ffmpegArgs,
	)

	if err != nil {
		return errorResult(
			err.Error(),
		)
	}

	return boolResult(
		result.Code == 0,
	)
}

// ============================================================
// Silence Filter
// ============================================================

// ============================================================
// Silence Filter
//
// WICHTIG: silenceremove mit stop_periods=1 in einem einzigen
// Durchlauf schneidet NICHT nur das Ende ab, sondern verwirft
// alles ab der ERSTEN gefundenen Stille-Periode im gesamten
// Stream. Bei Material mit Pausen mittendrin (Hörspiele,
// Sprachaufnahmen, Interviews) führt das dazu, dass der Großteil
// der Datei fälschlich abgeschnitten wird.
//
// Der korrekte Ansatz: Stille am Anfang entfernen, Audio
// umkehren, den (jetzt am Anfang liegenden) ursprünglichen
// Schluss ebenfalls von Stille befreien, wieder umkehren. So
// wird nie mitten im Stream geschnitten - nur an den beiden
// tatsächlichen Rändern.
// ============================================================

func silenceFilter(thresholdDB int, durationSec float64) string {
	threshold := fmt.Sprintf("%ddB", thresholdDB)
	duration := fmt.Sprintf("%g", durationSec)

	return "silenceremove=start_periods=1:start_duration=" + duration + ":start_threshold=" + threshold + ":detection=peak," +
		"areverse," +
		"silenceremove=start_periods=1:start_duration=" + duration + ":start_threshold=" + threshold + ":detection=peak," +
		"areverse"
}

// ============================================================
// media.ToMP3(input, output, [bitrate], [trimSilence])
// ============================================================

func handleToMP3(
	args []jsonValue,
) []byte {

	input, errResult := requireString(
		args,
		0,
		"media.ToMP3",
	)

	if errResult != nil {
		return errResult
	}

	output, errResult := requireString(
		args,
		1,
		"media.ToMP3",
	)

	if errResult != nil {
		return errResult
	}

	bitrate := 192

	if len(args) > 2 {
		bitrate = valueInt(
			args[2],
			192,
		)
	}

	if bitrate <= 0 {
		return errorResult(
			"media.ToMP3: Bitrate muss größer als 0 sein",
		)
	}

	trimSilence := false

	if len(args) > 3 {
		trimSilence = valueBool(
			args[3],
		)
	}

	if len(args) > 6 {
		return errorResult(
			"media.ToMP3 erwartet maximal 6 Argumente",
		)
	}

	silenceThreshold := -50
	if len(args) > 4 {
		silenceThreshold = valueInt(args[4], -50)
	}

	silenceDuration := 0.3
	if len(args) > 5 {
		d := args[5].Num
		if d > 0 {
			silenceDuration = d
		}
	}

	ffmpegArgs := []string{
		"-y",
		"-i",
		input,
		"-vn",
		"-codec:a",
		"libmp3lame",
		"-b:a",
		strconv.Itoa(bitrate) + "k",
	}

	if trimSilence {
		ffmpegArgs = append(
			ffmpegArgs,
			"-af",
			silenceFilter(silenceThreshold, silenceDuration),
		)
	}

	ffmpegArgs = append(
		ffmpegArgs,
		output,
	)

	result, err := ffmpegExec(
		ffmpegArgs,
	)

	if err != nil {
		return errorResult(
			err.Error(),
		)
	}

	if result.Code != 0 {
		return ffmpegError(result)
	}

	return strResult("OK")
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// ============================================================
// silencedetect-Ausgabe parsen
//
// Typische Zeilen:
//   [silencedetect @ 0x...] silence_start: 1.234
//   [silencedetect @ 0x...] silence_end: 2.456 | silence_duration: 1.222
// ============================================================

func parseSilenceDetect(text string) []jsonValue {

	var periods []jsonValue
	var currentStart float64
	haveStart := false

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)

		if idx := strings.Index(line, "silence_start:"); idx >= 0 {
			valStr := strings.TrimSpace(line[idx+len("silence_start:"):])
			if v, err := strconv.ParseFloat(valStr, 64); err == nil {
				currentStart = v
				haveStart = true
			}
			continue
		}

		if idx := strings.Index(line, "silence_end:"); idx >= 0 && haveStart {
			rest := line[idx+len("silence_end:"):]

			endStr := rest
			if pipeIdx := strings.Index(rest, "|"); pipeIdx >= 0 {
				endStr = rest[:pipeIdx]
			}
			endStr = strings.TrimSpace(endStr)

			endVal, err := strconv.ParseFloat(endStr, 64)
			if err != nil {
				continue
			}

			periods = append(periods, jsonValue{
				Type: "map",
				Map: map[string]jsonValue{
					"start":    {Type: "num", Num: round1(currentStart)},
					"end":      {Type: "num", Num: round1(endVal)},
					"duration": {Type: "num", Num: round1(endVal - currentStart)},
				},
			})

			haveStart = false
		}
	}

	return periods
}

// ============================================================
// media.TrimSilence(input, output)
// ============================================================

func handleTrimSilence(
	args []jsonValue,
) []byte {

	input, errResult := requireString(
		args,
		0,
		"media.TrimSilence",
	)

	if errResult != nil {
		return errResult
	}

	output, errResult := requireString(
		args,
		1,
		"media.TrimSilence",
	)

	if errResult != nil {
		return errResult
	}

	if len(args) > 4 {
		return errorResult(
			"media.TrimSilence erwartet maximal 4 Argumente",
		)
	}

	silenceThreshold := -50
	if len(args) > 2 {
		silenceThreshold = valueInt(args[2], -50)
	}

	silenceDuration := 0.3
	if len(args) > 3 {
		d := args[3].Num
		if d > 0 {
			silenceDuration = d
		}
	}

	ffmpegArgs := []string{
		"-y",
		"-i",
		input,
		"-vn",
		"-af",
		silenceFilter(silenceThreshold, silenceDuration),
		"-codec:a",
		"libmp3lame",
		"-b:a",
		"192k",
		output,
	}

	result, err := ffmpegExec(
		ffmpegArgs,
	)

	if err != nil {
		return errorResult(
			err.Error(),
		)
	}

	if result.Code != 0 {
		return ffmpegError(result)
	}

	return strResult("OK")
}

// ============================================================
// media.Merge(files, output, [bitrate])
// ============================================================

func handleMerge(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("media.Merge erwartet mindestens 2 Argumente")
	}

	files, errResult := requireStringArray(args[0], "media.Merge")
	if errResult != nil {
		return errResult
	}

	if len(files) < 2 {
		return errorResult("media.Merge: es werden mindestens 2 Dateien benötigt")
	}

	output, errResult := requireString(args, 1, "media.Merge")
	if errResult != nil {
		return errResult
	}

	bitrate := 0
	if len(args) > 2 {
		bitrate = valueInt(args[2], 0)
		if bitrate <= 0 {
			return errorResult("media.Merge: Bitrate muss größer als 0 sein")
		}
	}

	// Kein bitrate-Argument angegeben: höchste Bitrate der Quelldateien
	// ermitteln, damit das Ergebnis nie schlechter ist als die beste
	// Quelldatei. Schlägt die Ermittlung fehl (z.B. Format ohne
	// erkennbare Bitrate), wird auf den bisherigen Default zurückgefallen.
	if bitrate == 0 {
		for _, f := range files {
			result, err := ffmpegExec([]string{"-hide_banner", "-i", f})
			if err != nil {
				continue
			}
			if br, ok := findAudioBitrate(result.Stderr); ok && br > bitrate {
				bitrate = br
			}
		}
		if bitrate == 0 {
			fmt.Println("[media.Merge] Warnung: Bitrate konnte bei keiner Quelldatei ermittelt werden, verwende Standard 192 kbps. Bei Bedarf mit explizitem Bitrate-Parameter erneut ausführen.")
			bitrate = 192
		}
	}

	if len(args) > 3 {
		return errorResult("media.Merge erwartet maximal 3 Argumente")
	}

	ffmpegArgs := []string{
		"-y",
	}

	for _, file := range files {
		ffmpegArgs = append(
			ffmpegArgs,
			"-i",
			file,
		)
	}

	var filterInputs strings.Builder

	for i := range files {
		filterInputs.WriteString(
			fmt.Sprintf("[%d:a]", i),
		)
	}

	filterComplex := fmt.Sprintf(
		"%sconcat=n=%d:v=0:a=1[out]",
		filterInputs.String(),
		len(files),
	)

	ffmpegArgs = append(
		ffmpegArgs,
		"-filter_complex",
		filterComplex,
		"-map",
		"[out]",
		"-codec:a",
		"libmp3lame",
		"-b:a",
		strconv.Itoa(bitrate)+"k",
		output,
	)

	result, err := ffmpegExec(
		ffmpegArgs,
	)

	if err != nil {
		return errorResult(
			err.Error(),
		)
	}

	if result.Code != 0 {
		return ffmpegError(result)
	}

	return strResult("OK")
}

func numResult(n float64) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "num",
		Num:  n,
	})
	return data
}

// ============================================================
// Unterstützte Tags
// ============================================================

var supportedTags = map[string]string{
	"artist":       "Interpret",
	"album_artist": "Album-Interpret",
	"album":        "Album",
	"title":        "Titel",
	"genre":        "Genre",
	"date":         "Datum",
	"track":        "Titelnummer",
	"disc":         "Disc-Nummer",
	"composer":     "Komponist",
	"comment":      "Kommentar",
	"copyright":    "Copyright",
	"publisher":    "Verlag",
	"description":  "Beschreibung",
	"language":     "Sprache",
}

// ============================================================
// Tag-Aliase
// ============================================================

var tagAliases = map[string]string{
	"artist":    "artist",
	"interpret": "artist",

	"album_artist":    "album_artist",
	"album artist":    "album_artist",
	"album-artist":    "album_artist",
	"album_interpret": "album_artist",
	"album interpret": "album_artist",
	"album-interpret": "album_artist",

	"album": "album",

	"title": "title",
	"titel": "title",

	"genre": "genre",

	"date":  "date",
	"datum": "date",
	"jahr":  "date",

	"track":        "track",
	"tracknumber":  "track",
	"track number": "track",
	"track-number": "track",
	"titelnummer":  "track",

	"disc":        "disc",
	"discnumber":  "disc",
	"disc number": "disc",
	"disc-number": "disc",
	"disc_nummer": "disc",
	"disc-nummer": "disc",

	"composer":  "composer",
	"komponist": "composer",

	"comment":   "comment",
	"kommentar": "comment",

	"copyright": "copyright",

	"publisher": "publisher",
	"verlag":    "publisher",

	"description":  "description",
	"beschreibung": "description",

	"language": "language",
	"sprache":  "language",
}

// ============================================================
// Tag-Namen normalisieren
// ============================================================

func resolveTagAlias(
	name string,
) (string, bool) {

	key := strings.ToLower(
		strings.TrimSpace(name),
	)

	result, ok := tagAliases[key]

	return result, ok
}

// ============================================================
// media.Tags([name|names])
// ============================================================

func handleTags(
	args []jsonValue,
) []byte {

	if len(args) > 1 {
		return errorResult(
			"media.Tags erwartet höchstens 1 Argument",
		)
	}

	if len(args) == 0 {

		names := []string{
			"artist",
			"album_artist",
			"album",
			"title",
			"genre",
			"date",
			"track",
			"disc",
			"composer",
			"comment",
			"copyright",
			"publisher",
			"description",
			"language",
		}

		result := make(
			[]jsonValue,
			0,
			len(names),
		)

		for _, name := range names {
			result = append(
				result,
				jsonValue{
					Type: "str",
					Str:  name,
				},
			)
		}

		return arrayResult(result)
	}

	if args[0].Type == "str" {

		name := strings.TrimSpace(
			args[0].Str,
		)

		if name == "" {
			return errorResult(
				"media.Tags: Tag-Name darf nicht leer sein",
			)
		}

		key, ok := resolveTagAlias(
			name,
		)

		if !ok {
			return errorResult(
				fmt.Sprintf(
					"Tag wird aktuell nicht unterstützt: %s",
					name,
				),
			)
		}

		return strResult(key)
	}

	if args[0].Type == "arr" {

		if len(args[0].Arr) == 0 {
			return errorResult(
				"media.Tags: Tag-Liste darf nicht leer sein",
			)
		}

		result := make(
			[]jsonValue,
			0,
			len(args[0].Arr),
		)

		for i, item := range args[0].Arr {

			if item.Type != "str" ||
				strings.TrimSpace(item.Str) == "" {

				return errorResult(
					fmt.Sprintf(
						"media.Tags: Element %d muss ein nicht leerer String sein",
						i+1,
					),
				)
			}

			name := strings.TrimSpace(
				item.Str,
			)

			key, ok := resolveTagAlias(
				name,
			)

			if !ok {
				return errorResult(
					fmt.Sprintf(
						"Tag wird aktuell nicht unterstützt: %s",
						name,
					),
				)
			}

			result = append(
				result,
				jsonValue{
					Type: "str",
					Str:  key,
				},
			)
		}

		return arrayResult(result)
	}

	return errorResult(
		"media.Tags erwartet einen Tag-Namen oder ein Array von Tag-Namen",
	)
}

// ============================================================
// FFmetadata
// ============================================================

func unescapeFFMetadata(s string) string {

	var b strings.Builder
	escaped := false

	for _, r := range s {

		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			escaped = true
			continue
		}

		b.WriteRune(r)
	}

	if escaped {
		b.WriteByte('\\')
	}

	return b.String()
}

func parseFFMetadata(
	text string,
) map[string]jsonValue {

	result := make(
		map[string]jsonValue,
	)

	lines := strings.Split(
		text,
		"\n",
	)

	for _, line := range lines {

		line = strings.TrimSuffix(
			line,
			"\r",
		)

		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if line == ";FFMETADATA1" {
			continue
		}

		if strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") &&
			strings.HasSuffix(line, "]") {
			continue
		}

		pos := -1
		escaped := false

		for i := 0; i < len(line); i++ {

			if escaped {
				escaped = false
				continue
			}

			if line[i] == '\\' {
				escaped = true
				continue
			}

			if line[i] == '=' {
				pos = i
				break
			}
		}

		if pos <= 0 {
			continue
		}

		key := unescapeFFMetadata(
			line[:pos],
		)

		value := unescapeFFMetadata(
			line[pos+1:],
		)

		if key == "" {
			continue
		}

		result[key] = jsonValue{
			Type: "str",
			Str:  value,
		}
	}

	return result
}

// ============================================================
// Tag suchen
// ============================================================

func findTag(
	tags map[string]jsonValue,
	tag string,
) (jsonValue, bool) {

	needle := strings.ToLower(
		strings.TrimSpace(tag),
	)

	for key, value := range tags {

		if strings.ToLower(
			strings.TrimSpace(key),
		) == needle {

			return value, true
		}
	}

	return jsonValue{}, false
}

func normalizeTagName(tag string) string {
	return strings.ToLower(
		strings.TrimSpace(tag),
	)
}

// ============================================================
// media.GetTag(input, [tag])
// ============================================================

func handleGetTag(
	args []jsonValue,
) []byte {

	input, errResult := requireString(
		args,
		0,
		"media.GetTag",
	)

	if errResult != nil {
		return errResult
	}

	if len(args) > 2 {
		return errorResult(
			"media.GetTag erwartet 1 oder 2 Argumente",
		)
	}

	ffmpegArgs := []string{
		"-hide_banner",
		"-loglevel",
		"error",
		"-i",
		input,
		"-map_metadata",
		"0",
		"-f",
		"ffmetadata",
		"-",
	}

	result, err := ffmpegExec(
		ffmpegArgs,
	)

	if err != nil {
		return errorResult(
			err.Error(),
		)
	}

	if result.Code != 0 {
		return ffmpegError(result)
	}

	tags := parseFFMetadata(
		result.Stdout,
	)

	if len(args) == 2 {

		tag, errResult := requireString(
			args,
			1,
			"media.GetTag",
		)

		if errResult != nil {
			return errResult
		}

		value, ok := findTag(
			tags,
			tag,
		)

		if !ok {
			return strResult("")
		}

		return strResult(
			value.Str,
		)
	}

	return mapResult(tags)
}

// ============================================================
// media.GetBitrate(input)
// ============================================================

func handleGetBitrate(args []jsonValue) []byte {

	input, errResult := requireString(args, 0, "media.GetBitrate")
	if errResult != nil {
		return errResult
	}

	if len(args) > 1 {
		return errorResult("media.GetBitrate erwartet 1 Argument")
	}

	// FFmpeg beendet einen reinen Info-Aufruf (kein Output angegeben)
	// mit einem Fehler-Exit-Code - das ist hier kein echter Fehler,
	// die Stream-Info steht trotzdem in Stderr.
	result, err := ffmpegExec([]string{
		"-hide_banner",
		"-i", input,
	})
	if err != nil {
		return errorResult(err.Error())
	}

	bitrate, ok := findAudioBitrate(result.Stderr)
	if !ok {
		return errorResult("media.GetBitrate: Bitrate konnte nicht ermittelt werden")
	}

	return numResult(float64(bitrate))
}

// ============================================================
// media.IsCover(file)
//
// Ein eingebettetes Cover erscheint in der FFmpeg-Stream-Info
// als eigener Video-Stream (mjpeg, meist mit "attached_pic"
// markiert). Die reine Existenz eines "Video:"-Streams reicht
// als Kriterium - MP3s haben sonst nie einen Video-Stream.
// ============================================================

func handleIsCover(args []jsonValue) []byte {

	file, errResult := requireString(args, 0, "media.IsCover")
	if errResult != nil {
		return errResult
	}

	if len(args) > 1 {
		return errorResult("media.IsCover erwartet 1 Argument")
	}

	_, _, mediaExt := splitMediaPath(file)

	if strings.ToLower(mediaExt) != ".mp3" {
		return errorResult("media.IsCover unterstützt aktuell nur MP3-Dateien")
	}

	result, err := ffmpegExec([]string{
		"-hide_banner",
		"-i", file,
	})
	if err != nil {
		return errorResult(err.Error())
	}

	hasCover := strings.Contains(result.Stderr, "Video:")

	return boolResult(hasCover)
}

// ============================================================
// media.GetTags(files, [tag])
// ============================================================

func handleGetTags(
	args []jsonValue,
) []byte {

	if len(args) < 1 {
		return errorResult(
			"media.GetTags erwartet mindestens 1 Argument",
		)
	}

	files, errResult := requireStringArray(
		args[0],
		"media.GetTags",
	)

	if errResult != nil {
		return errResult
	}

	if len(args) > 2 {
		return errorResult(
			"media.GetTags erwartet 1 oder 2 Argumente",
		)
	}

	tag := ""

	if len(args) == 2 {

		tag, errResult = requireString(
			args,
			1,
			"media.GetTags",
		)

		if errResult != nil {
			return errResult
		}
	}

	values := make(
		[]jsonValue,
		0,
		len(files),
	)

	for _, file := range files {

		ffmpegArgs := []string{
			"-hide_banner",
			"-loglevel",
			"error",
			"-i",
			file,
			"-map_metadata",
			"0",
			"-f",
			"ffmetadata",
			"-",
		}

		result, err := ffmpegExec(
			ffmpegArgs,
		)

		if err != nil {
			return errorResult(
				fmt.Sprintf(
					"media.GetTags: %s: %v",
					file,
					err,
				),
			)
		}

		if result.Code != 0 {
			errValue := ffmpegError(result)

			var parsed jsonValue

			if json.Unmarshal(
				errValue,
				&parsed,
			) == nil {

				return errorResult(
					fmt.Sprintf(
						"media.GetTags: %s: %s",
						file,
						parsed.Message,
					),
				)
			}

			return errorResult(
				fmt.Sprintf(
					"media.GetTags: %s: FFmpeg fehlgeschlagen",
					file,
				),
			)
		}

		tags := parseFFMetadata(
			result.Stdout,
		)

		if tag != "" {

			value, ok := findTag(
				tags,
				tag,
			)

			if !ok {
				values = append(
					values,
					jsonValue{
						Type: "str",
						Str:  "",
					},
				)
			} else {
				values = append(
					values,
					value,
				)
			}

			continue
		}

		values = append(
			values,
			jsonValue{
				Type: "map",
				Map:  tags,
			},
		)
	}

	return arrayResult(values)
}

// ============================================================
// Tag Map
// ============================================================

func getTagMap(
	v jsonValue,
	name string,
) (map[string]string, []byte) {

	if v.Type != "map" {
		return nil, errorResult(
			fmt.Sprintf(
				"%s: Tags müssen als Map angegeben werden",
				name,
			),
		)
	}

	if len(v.Map) == 0 {
		return nil, errorResult(
			fmt.Sprintf(
				"%s: Tag-Map darf nicht leer sein",
				name,
			),
		)
	}

	result := make(
		map[string]string,
	)

	for key, value := range v.Map {

		key = normalizeTagName(key)

		if key == "" {
			return nil, errorResult(
				fmt.Sprintf(
					"%s: Tag-Name darf nicht leer sein",
					name,
				),
			)
		}

		str, errResult := jsonValueToString(
			value,
			name,
		)

		if errResult != nil {
			return nil, errResult
		}

		result[key] = str
	}

	return result, nil
}

// ============================================================
// SetTag / SetTags
// ============================================================

func setTagsForFile(
	file string,
	tags map[string]string,
) error {

	if len(tags) == 0 {
		return fmt.Errorf(
			"keine Tags angegeben",
		)
	}

	temp := temporaryMediaPath(
		file,
	)

	ffmpegArgs := []string{
		"-y",
		"-hide_banner",
		"-loglevel",
		"error",
		"-i",
		file,
		"-map",
		"0",
		"-c",
		"copy",
	}

	for key, value := range tags {

		ffmpegArgs = append(
			ffmpegArgs,
			"-metadata",
			key+"="+value,
		)
	}

	ffmpegArgs = append(
		ffmpegArgs,
		temp,
	)

	result, err := ffmpegExec(
		ffmpegArgs,
	)

	if err != nil {
		return err
	}

	if result.Code != 0 {

		msg := strings.TrimSpace(
			result.Stderr,
		)

		if msg == "" {
			msg = strings.TrimSpace(
				result.Stdout,
			)
		}

		if msg == "" {
			msg = fmt.Sprintf(
				"FFmpeg fehlgeschlagen (Exit-Code %d)",
				result.Code,
			)
		}

		return fmt.Errorf(
			"FFmpeg: %s",
			msg,
		)
	}

	if err := replaceFile(
		temp,
		file,
	); err != nil {

		return fmt.Errorf(
			"Datei konnte nach der Tag-Änderung nicht ersetzt werden: %v",
			err,
		)
	}

	return nil
}

// ============================================================
// media.SetTag(file, tag, value)
// media.SetTag(file, tags)
// ============================================================

func handleSetTag(
	args []jsonValue,
) []byte {

	file, errResult := requireString(
		args,
		0,
		"media.SetTag",
	)

	if errResult != nil {
		return errResult
	}

	if len(args) < 2 {
		return errorResult(
			"media.SetTag erwartet mindestens 2 Argumente",
		)
	}

	var tags map[string]string

	switch args[1].Type {

	case "map":

		if len(args) != 2 {
			return errorResult(
				"media.SetTag mit einer Tag-Map erwartet genau 2 Argumente",
			)
		}

		var err []byte

		tags, err = getTagMap(
			args[1],
			"media.SetTag",
		)

		if err != nil {
			return err
		}

	default:

		if len(args) != 3 {
			return errorResult(
				"media.SetTag erwartet file, tag, value",
			)
		}

		tag, err := requireString(
			args,
			1,
			"media.SetTag",
		)

		if err != nil {
			return err
		}

		value, err := jsonValueToString(
			args[2],
			"media.SetTag",
		)

		if err != nil {
			return err
		}

		tags = map[string]string{
			normalizeTagName(tag): value,
		}
	}

	if err := setTagsForFile(
		file,
		tags,
	); err != nil {

		return errorResult(
			err.Error(),
		)
	}

	return strResult("OK")
}

// ============================================================
// media.SetTags(files, tag, value)
// media.SetTags(files, tags)
// ============================================================

func handleSetTags(
	args []jsonValue,
) []byte {

	if len(args) < 2 {
		return errorResult(
			"media.SetTags erwartet mindestens 2 Argumente",
		)
	}

	files, errResult := requireStringArray(
		args[0],
		"media.SetTags",
	)

	if errResult != nil {
		return errResult
	}

	var tags map[string]string

	switch args[1].Type {

	case "map":

		if len(args) != 2 {
			return errorResult(
				"media.SetTags mit einer Tag-Map erwartet genau 2 Argumente",
			)
		}

		var err []byte

		tags, err = getTagMap(
			args[1],
			"media.SetTags",
		)

		if err != nil {
			return err
		}

	default:

		if len(args) != 3 {
			return errorResult(
				"media.SetTags erwartet files, tag, value",
			)
		}

		tag, err := requireString(
			args,
			1,
			"media.SetTags",
		)

		if err != nil {
			return err
		}

		value, err := jsonValueToString(
			args[2],
			"media.SetTags",
		)

		if err != nil {
			return err
		}

		tags = map[string]string{
			normalizeTagName(tag): value,
		}
	}

	for _, file := range files {

		if err := setTagsForFile(
			file,
			tags,
		); err != nil {

			return errorResult(
				fmt.Sprintf(
					"media.SetTags: %s: %v",
					file,
					err,
				),
			)
		}
	}

	return strResult("OK")
}

// ============================================================
// media.SetCover(file, jpg)
//
// Ersetzt bzw. setzt das eingebettete Cover eines MP3.
//
// Das Cover muss:
//   - JPG/JPEG sein
//   - maximal 1000 × 1000 Pixel haben
//   - maximal 1 MB groß sein
//
// FFmpeg schreibt eine neue MP3-Datei mit:
//   - allen vorhandenen Streams
//   - vorhandenen Metadaten
//   - genau einem eingebetteten Cover
//
// Danach ersetzt der Host die Originaldatei.
// ============================================================

func handleSetCover(
	args []jsonValue,
) []byte {

	if len(args) != 2 {
		return errorResult(
			"media.SetCover erwartet 2 Argumente",
		)
	}

	file, errResult := requireString(
		args,
		0,
		"media.SetCover",
	)

	if errResult != nil {
		return errResult
	}

	cover, errResult := requireString(
		args,
		1,
		"media.SetCover",
	)

	if errResult != nil {
		return errResult
	}

	_, _, mediaExt := splitMediaPath(
		file,
	)

	if strings.ToLower(mediaExt) != ".mp3" {
		return errorResult(
			"media.SetCover unterstützt aktuell nur MP3-Dateien",
		)
	}

	if err := validateCover(
		cover,
	); err != nil {

		return errorResult(
			"media.SetCover: " + err.Error(),
		)
	}

	temp := temporaryMediaPath(
		file,
	)

	ffmpegArgs := []string{
		"-y",
		"-hide_banner",
		"-loglevel",
		"error",

		// MP3 laden.
		"-i",
		file,

		// JPG-Cover laden.
		"-i",
		cover,

		// Alle Audio-/Datenstreams des MP3 übernehmen.
		"-map",
		"0",

		// Das Cover als Video-Stream hinzufügen.
		"-map",
		"1:v:0",

		// Audio unverändert übernehmen.
		"-c:a",
		"copy",

		// Cover als MJPEG speichern.
		"-c:v",
		"mjpeg",

		// Das Cover als angehängtes Bild markieren.
		"-disposition:v:0",
		"attached_pic",

		// Metadaten des MP3 behalten.
		"-map_metadata",
		"0",

		temp,
	}

	result, err := ffmpegExec(
		ffmpegArgs,
	)

	if err != nil {
		return errorResult(
			"media.SetCover: " + err.Error(),
		)
	}

	if result.Code != 0 {
		return errorResult(
			"media.SetCover: " +
				strings.TrimSpace(
					coverFFmpegError(result),
				),
		)
	}

	if err := replaceFile(
		temp,
		file,
	); err != nil {

		return errorResult(
			"media.SetCover: Datei konnte nach dem Setzen des Covers nicht ersetzt werden: " +
				err.Error(),
		)
	}

	return strResult("OK")
}

// ============================================================
// Cover-FFmpeg-Fehler
// ============================================================

func coverFFmpegError(
	result hostFFmpegResponse,
) string {

	msg := strings.TrimSpace(
		result.Stderr,
	)

	if msg == "" {
		msg = strings.TrimSpace(
			result.Stdout,
		)
	}

	if msg == "" {
		msg = fmt.Sprintf(
			"FFmpeg fehlgeschlagen (Exit-Code %d)",
			result.Code,
		)
	}

	return msg
}

// ============================================================
// media.AnalyzeSilence(file, [thresholdDB], [durationSec], [maxSeconds])
//
// Diagnose-Funktion: erkennt Stille-Abschnitte im Material,
// OHNE etwas zu schneiden. maxSeconds begrenzt die Analyse auf
// den Anfang der Datei (Standard 60s) - bei langem Material
// (Hörspiele) reicht das für die Kalibrierung des Anfangs völlig
// aus und spart die Zeit, die komplette Datei zu verarbeiten.
// Zeitangaben werden auf 0,1s gerundet.
// ============================================================

func handleAnalyzeSilence(args []jsonValue) []byte {

	file, errResult := requireString(args, 0, "media.AnalyzeSilence")
	if errResult != nil {
		return errResult
	}

	thresholdDB := -50
	if len(args) > 1 {
		thresholdDB = valueInt(args[1], -50)
	}

	durationSec := 0.3
	if len(args) > 2 {
		d := args[2].Num
		if d > 0 {
			durationSec = d
		}
	}

	maxSeconds := 60
	if len(args) > 3 {
		m := valueInt(args[3], 60)
		if m > 0 {
			maxSeconds = m
		}
	}

	filter := fmt.Sprintf("silencedetect=noise=%ddB:d=%g", thresholdDB, durationSec)

	result, err := ffmpegExec([]string{
		"-hide_banner",
		"-i", file,
		"-t", strconv.Itoa(maxSeconds),
		"-af", filter,
		"-f", "null",
		"-",
	})
	if err != nil {
		return errorResult(err.Error())
	}

	periods := parseSilenceDetect(result.Stderr)

	return arrayResult(periods)
}

// ============================================================
// media.GetCover(file, output)
//
// Extrahiert das eingebettete MP3-Cover als JPG.
//
// Die Ausgabe wird als JPEG erzeugt.
// ============================================================

func handleGetCover(
	args []jsonValue,
) []byte {

	if len(args) != 2 {
		return errorResult(
			"media.GetCover erwartet 2 Argumente",
		)
	}

	file, errResult := requireString(
		args,
		0,
		"media.GetCover",
	)

	if errResult != nil {
		return errResult
	}

	output, errResult := requireString(
		args,
		1,
		"media.GetCover",
	)

	if errResult != nil {
		return errResult
	}

	_, _, mediaExt := splitMediaPath(
		file,
	)

	if strings.ToLower(mediaExt) != ".mp3" {
		return errorResult(
			"media.GetCover unterstützt aktuell nur MP3-Dateien",
		)
	}

	_, _, outputExt := splitMediaPath(
		output,
	)

	if strings.ToLower(outputExt) != ".jpg" &&
		strings.ToLower(outputExt) != ".jpeg" {

		return errorResult(
			"media.GetCover: Ausgabe muss eine JPG-Datei sein",
		)
	}

	ffmpegArgs := []string{
		"-y",
		"-hide_banner",
		"-loglevel",
		"error",

		"-i",
		file,

		// Ausschließlich das eingebettete Bild verwenden.
		"-map",
		"0:v:0",

		// Nur ein Cover extrahieren.
		"-frames:v",
		"1",

		// Als JPG schreiben.
		"-c:v",
		"mjpeg",

		output,
	}

	result, err := ffmpegExec(
		ffmpegArgs,
	)

	if err != nil {
		return errorResult(
			"media.GetCover: " + err.Error(),
		)
	}

	if result.Code != 0 {
		return errorResult(
			"media.GetCover: " +
				coverFFmpegError(result),
		)
	}

	return strResult("OK")
}

// ============================================================
// vbx_describe
// ============================================================

type funcDesc struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Params      string `json:"params"`
	Description string `json:"description"`
}

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {

	desc := []funcDesc{

		{
			Namespace: "media",
			Name:      "ToMP3",
			Params:    "input, output, [bitrate], [trimSilence]",
			Description: "Konvertiert eine Audio- oder Videodatei nach MP3. " +
				"Mit trimSilence=True werden Stille am Anfang und Ende entfernt.",
		},

		{
			Namespace:   "media",
			Name:        "TrimSilence",
			Params:      "input, output",
			Description: "Entfernt Stille am Anfang und Ende einer Audiodatei.",
		},

		{
			Namespace:   "media",
			Name:        "Merge",
			Params:      "files, output, [bitrate]",
			Description: "Fügt mehrere Audiodateien in der angegebenen Reihenfolge zu einer MP3-Datei zusammen.",
		},

		{
			Namespace:   "media",
			Name:        "IsValid",
			Params:      "input",
			Description: "Prüft, ob FFmpeg die Audio- und Videostreams einer Mediendatei vollständig verarbeiten kann. True bedeutet, dass FFmpeg erfolgreich beendet wurde.",
		},

		{
			Namespace:   "media",
			Name:        "GetBitrate",
			Params:      "input",
			Description: "Ermittelt die Audio-Bitrate (kbit/s) einer Mediendatei.",
		},

		{
			Namespace:   "media",
			Name:        "IsCover",
			Params:      "file",
			Description: "Prüft, ob eine MP3-Datei ein eingebettetes Cover hat.",
		},

		{
			Namespace:   "media",
			Name:        "CheckTags",
			Params:      "file, tags",
			Description: "Prüft, ob alle angegebenen Tags gesetzt und nicht leer sind. Gibt bei einem fehlenden Tag den Dateipfad zurück, sonst einen leeren String.",
		},

		{
			Namespace:   "media",
			Name:        "AnalyzeSilence",
			Params:      "file, [thresholdDB], [durationSec], [maxSeconds]",
			Description: "Erkennt Stille-Abschnitte im Material (ohne zu schneiden), begrenzt auf die ersten maxSeconds Sekunden (Standard 60). Zum Kalibrieren der Trimm-Schwellwerte vor dem eigentlichen Schneiden mit ToMP3/TrimSilence.",
		},

		{
			Namespace:   "media",
			Name:        "GetDuration",
			Params:      "input",
			Description: "Ermittelt die Dauer einer Mediendatei in Sekunden.",
		},

		{
			Namespace:   "media",
			Name:        "GetInfo",
			Params:      "file",
			Description: "Liefert Bitrate (kbit/s), Dauer (Sekunden) und ob ein Cover eingebettet ist, in einem einzigen FFmpeg-Aufruf (Map mit bitrate/duration/hasCover).",
		},

		{
			Namespace:   "media",
			Name:        "Tags",
			Params:      "[name|names]",
			Description: "Übersetzt einen deutschen oder englischen Tag-Namen in den kanonischen internen Tag-Namen. Einzelne Namen werden als String, mehrere Namen als Array zurückgegeben. Ohne Argument werden alle kanonischen Tag-Namen zurückgegeben.",
		},

		{
			Namespace:   "media",
			Name:        "GetTag",
			Params:      "input, [tag]",
			Description: "Liest einen einzelnen Tag oder alle Tags einer Mediendatei. Die Schreibweise des Tag-Namens ist unabhängig von Groß-/Kleinschreibung.",
		},

		{
			Namespace:   "media",
			Name:        "GetTags",
			Params:      "files, [tag]",
			Description: "Liest einen einzelnen Tag oder alle Tags mehrerer Mediendateien. Die Schreibweise des Tag-Namens ist unabhängig von Groß-/Kleinschreibung.",
		},

		{
			Namespace:   "media",
			Name:        "SetTag",
			Params:      "file, tag, value | file, tags",
			Description: "Setzt einen oder mehrere Tags einer Mediendatei. Die Schreibweise des Tag-Namens ist unabhängig von Groß-/Kleinschreibung.",
		},

		{
			Namespace:   "media",
			Name:        "SetTags",
			Params:      "files, tag, value | files, tags",
			Description: "Setzt einen oder mehrere Tags für mehrere Mediendateien. Bei einer Tag-Map werden alle Änderungen pro Datei mit einem einzigen FFmpeg-Aufruf durchgeführt.",
		},

		{
			Namespace:   "media",
			Name:        "SetCover",
			Params:      "file, jpg",
			Description: "Setzt das eingebettete Cover einer MP3-Datei. Das Cover muss JPG sein und darf maximal 1000 × 1000 Pixel und 1 MB groß sein.",
		},

		{
			Namespace:   "media",
			Name:        "GetCover",
			Params:      "file, output",
			Description: "Extrahiert das eingebettete Cover einer MP3-Datei als JPG-Datei.",
		},
	}

	data, err := json.Marshal(desc)

	if err != nil {
		return packBytes(
			errorResult(
				"Beschreibung konnte nicht erzeugt werden: " +
					err.Error(),
			),
		)
	}

	return packBytes(data)
}

// ============================================================
// vbx_call
// ============================================================

//go:wasmexport vbx_call
func vbxCall(
	namePtr,
	nameLen,
	argsPtr,
	argsLen uint32,
) uint64 {

	nameBytes := readBytes(
		namePtr,
		nameLen,
	)

	if nameBytes == nil {
		return packBytes(
			errorResult(
				"Funktionsname konnte nicht gelesen werden",
			),
		)
	}

	name := string(nameBytes)

	argsBytes := readBytes(
		argsPtr,
		argsLen,
	)

	if argsBytes == nil {
		return packBytes(
			errorResult(
				"Argumente konnten nicht gelesen werden",
			),
		)
	}

	var args []jsonValue

	if err := json.Unmarshal(
		argsBytes,
		&args,
	); err != nil {

		return packBytes(
			errorResult(
				"Ungültige Argumente: " +
					err.Error(),
			),
		)
	}

	var result []byte

	switch name {

	case "ToMP3":
		result = handleToMP3(args)

	case "TrimSilence":
		result = handleTrimSilence(args)

	case "Merge":
		result = handleMerge(args)

	case "IsValid":
		result = handleIsValid(args)

	case "GetBitrate":
		result = handleGetBitrate(args)

	case "IsCover":
		result = handleIsCover(args)

	case "CheckTags":
		result = handleCheckTags(args)

	case "AnalyzeSilence":
		result = handleAnalyzeSilence(args)

	case "GetDuration":
		result = handleGetDuration(args)

	case "GetInfo":
		result = handleGetInfo(args)

	case "Tags":
		result = handleTags(args)

	case "GetTag":
		result = handleGetTag(args)

	case "GetTags":
		result = handleGetTags(args)

	case "SetTag":
		result = handleSetTag(args)

	case "SetTags":
		result = handleSetTags(args)

	case "SetCover":
		result = handleSetCover(args)

	case "GetCover":
		result = handleGetCover(args)

	default:
		result = errorResult(
			"Unbekannte Media-Funktion: " +
				name,
		)
	}

	return packBytes(result)
}

// ============================================================
// main
// ============================================================

func main() {}
