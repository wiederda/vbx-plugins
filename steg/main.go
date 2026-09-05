package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/image/bmp"
)

// ============================================================
// VBX Plugin ABI
// ============================================================

const abiVersion = 1

// ------------------------------------------------------------
// Speicherverwaltung
// ------------------------------------------------------------

var liveBuffers = map[uint32][]byte{}

func alloc(size uint32) uint32 {
	buf := make([]byte, size)

	// Auch bei size=0 brauchen wir einen gültigen Pointer.
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
// Plugin Helpers
// ============================================================

func makeString(value string) jsonValue {
	return jsonValue{
		Type: "str",
		Str:  value,
	}
}

func makeBool(value bool) jsonValue {
	return jsonValue{
		Type: "bool",
		Bool: value,
	}
}

func makeNum(value float64) jsonValue {
	return jsonValue{
		Type: "num",
		Num:  value,
	}
}

func makeEmpty() jsonValue {
	return jsonValue{
		Type: "empty",
	}
}

func makeError(err error) jsonValue {
	return jsonValue{
		Type:    "error",
		Message: err.Error(),
	}
}

func makeArray(values []jsonValue) jsonValue {
	return jsonValue{
		Type: "arr",
		Arr:  values,
	}
}

func makeCapacityError(err error) jsonValue {
	return makeArray([]jsonValue{
		makeBool(false),
		makeNum(0),
		makeString(err.Error()),
		makeNum(0),
	})
}

// ------------------------------------------------------------
// JSON Argumente lesen
// ------------------------------------------------------------

func readArgs(
	ptr uint32,
	length uint32,
) ([]jsonValue, error) {

	buf, ok := liveBuffers[ptr]

	if !ok {
		return nil, fmt.Errorf(
			"ungültiger Argument-Pointer: %d",
			ptr,
		)
	}

	if length > uint32(len(buf)) {
		return nil, fmt.Errorf(
			"ungültige Argument-Länge: %d",
			length,
		)
	}

	var args []jsonValue

	if err := json.Unmarshal(
		buf[:length],
		&args,
	); err != nil {
		return nil, fmt.Errorf(
			"ungültiges Argument-JSON: %w",
			err,
		)
	}

	return args, nil
}

func argString(
	args []jsonValue,
	index int,
) (string, error) {

	if index >= len(args) {
		return "", fmt.Errorf(
			"Argument %d fehlt",
			index+1,
		)
	}

	if args[index].Type != "str" {
		return "", fmt.Errorf(
			"Argument %d muss ein String sein",
			index+1,
		)
	}

	return args[index].Str, nil
}

func argNum(
	args []jsonValue,
	index int,
) (float64, error) {

	if index >= len(args) {
		return 0, fmt.Errorf(
			"Argument %d fehlt",
			index+1,
		)
	}

	if args[index].Type != "num" {
		return 0, fmt.Errorf(
			"Argument %d muss eine Zahl sein",
			index+1,
		)
	}

	return args[index].Num, nil
}

// ============================================================
// Pfade
// ============================================================

func normalizePath(path string) (string, error) {

	path = strings.TrimSpace(path)

	if path == "" {
		return "", fmt.Errorf(
			"kein Dateipfad angegeben",
		)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf(
			"Pfad konnte nicht aufgelöst werden: %w",
			err,
		)
	}

	return filepath.Clean(abs), nil
}

// ============================================================
// Steganografie
// ============================================================

// ------------------------------------------------------------
// Seed / Stride
// ------------------------------------------------------------

func strideFromSeed(seedStr string) int {

	h := sha256.Sum256(
		[]byte(seedStr),
	)

	return 2 + int(h[0]%3)
}

// ------------------------------------------------------------
// BMP erkennen
// ------------------------------------------------------------

func isBMP(data []byte) bool {

	return len(data) >= 2 &&
		data[0] == 'B' &&
		data[1] == 'M'
}

// ============================================================
// Bild-Dekodierung
// ============================================================

func decodeImageSafe(
	data []byte,
) (image.Image, string, error) {

	if isBMP(data) {

		img, err := bmp.Decode(
			strings.NewReader(
				string(data),
			),
		)

		if err != nil {
			return nil, "", fmt.Errorf(
				"BMP konnte nicht gelesen werden: %w",
				err,
			)
		}

		return img, "bmp", nil
	}

	img, format, err := image.Decode(
		strings.NewReader(
			string(data),
		),
	)

	if err != nil {
		return nil, "", fmt.Errorf(
			"Bild konnte nicht gelesen werden: %w",
			err,
		)
	}

	return img, strings.ToLower(format), nil
}

// ============================================================
// RGBA
// ============================================================

func toRGBA(
	img image.Image,
) *image.RGBA {

	bounds := img.Bounds()

	rgba := image.NewRGBA(
		bounds,
	)

	draw.Draw(
		rgba,
		bounds,
		img,
		bounds.Min,
		draw.Src,
	)

	return rgba
}

// ============================================================
// Payload
// ============================================================

func buildPayload(
	data []byte,
) []byte {

	payload := make(
		[]byte,
		8+len(data),
	)

	copy(
		payload[0:4],
		[]byte("STEG"),
	)

	binary.LittleEndian.PutUint32(
		payload[4:8],
		uint32(len(data)),
	)

	copy(
		payload[8:],
		data,
	)

	return payload
}

// ============================================================
// Bounds
// ============================================================

func checkBounds(
	availableUnits int,
	payloadBytes int,
	stride int,
) error {

	requiredBits := payloadBytes * 8

	requiredUnits := requiredBits * stride

	if requiredUnits > availableUnits {
		return fmt.Errorf(
			"nicht genügend Speicherplatz: benötigt %d Einheiten, verfügbar %d",
			requiredUnits,
			availableUnits,
		)
	}

	return nil
}

// ============================================================
// Sichere Permutation
// ============================================================

func securePermutation(
	n int,
	seedStr string,
) []int {

	result := make(
		[]int,
		n,
	)

	for i := range result {
		result[i] = i
	}

	key := sha256.Sum256(
		[]byte(seedStr),
	)

	block, err := aes.NewCipher(
		key[:],
	)

	if err != nil {
		return result
	}

	iv := make(
		[]byte,
		aes.BlockSize,
	)

	copy(
		iv,
		key[:aes.BlockSize],
	)

	stream := cipher.NewCTR(
		block,
		iv,
	)

	for i := n - 1; i > 0; i-- {

		r := nextUint64(stream)

		j := int(
			r % uint64(i+1),
		)

		result[i], result[j] =
			result[j], result[i]
	}

	return result
}

// ------------------------------------------------------------
// Zufallszahl aus CTR
// ------------------------------------------------------------

func nextUint64(
	s cipher.Stream,
) uint64 {

	var in [8]byte
	var out [8]byte

	s.XORKeyStream(
		out[:],
		in[:],
	)

	return binary.LittleEndian.Uint64(
		out[:],
	)
}

// ============================================================
// Bits
// ============================================================

func bytesToBits(
	data []byte,
) []byte {

	bits := make(
		[]byte,
		0,
		len(data)*8,
	)

	for _, b := range data {

		for i := 7; i >= 0; i-- {

			bits = append(
				bits,
				(b>>uint(i))&1,
			)
		}
	}

	return bits
}

func bitsToBytes(
	bits []byte,
) []byte {

	if len(bits)%8 != 0 {
		return nil
	}

	result := make(
		[]byte,
		len(bits)/8,
	)

	for i := range result {

		var b byte

		for j := 0; j < 8; j++ {

			b <<= 1
			b |= bits[i*8+j] & 1
		}

		result[i] = b
	}

	return result
}

// ============================================================
// BMP Core
// ============================================================

func injectCore(
	data []byte,
	payload []byte,
	seedStr string,
) error {

	if len(data) < 14 {
		return fmt.Errorf(
			"ungültige BMP-Datei",
		)
	}

	offset := int(
		binary.LittleEndian.Uint32(
			data[10:14],
		),
	)

	if offset < 14 || offset >= len(data) {
		return fmt.Errorf(
			"ungültiger BMP-Pixeloffset: %d",
			offset,
		)
	}

	stride := strideFromSeed(
		seedStr,
	)

	bits := bytesToBits(
		payload,
	)

	available := len(data) - offset

	if err := checkBounds(
		available,
		len(payload),
		stride,
	); err != nil {
		return err
	}

	positions := securePermutation(
		available,
		seedStr,
	)

	for bitIndex := 0; bitIndex < len(bits); bitIndex++ {

		posIndex := bitIndex * stride

		if posIndex >= len(positions) {
			return fmt.Errorf(
				"Payload unvollständig",
			)
		}

		pos := positions[posIndex]

		data[offset+pos] =
			(data[offset+pos] & 0xFE) |
				bits[bitIndex]
	}

	return nil
}

func extractCore(
	data []byte,
	seedStr string,
) ([]byte, error) {

	if len(data) < 14 {
		return nil, fmt.Errorf(
			"ungültige BMP-Datei",
		)
	}

	offset := int(
		binary.LittleEndian.Uint32(
			data[10:14],
		),
	)

	if offset < 14 || offset >= len(data) {
		return nil, fmt.Errorf(
			"ungültiger BMP-Pixeloffset: %d",
			offset,
		)
	}

	stride := strideFromSeed(
		seedStr,
	)

	available := len(data) - offset

	positions := securePermutation(
		available,
		seedStr,
	)

	availableBits := available / stride

	if availableBits < 64 {
		return nil, fmt.Errorf(
			"BMP enthält keine ausreichenden Daten",
		)
	}

	// --------------------------------------------------------
	// Header lesen
	// --------------------------------------------------------

	headerBits := make(
		[]byte,
		64,
	)

	for i := 0; i < 64; i++ {

		posIndex := i * stride

		if posIndex >= len(positions) {
			return nil, fmt.Errorf(
				"Payload unvollständig",
			)
		}

		pos := positions[posIndex]

		headerBits[i] =
			data[offset+pos] & 1
	}

	header := bitsToBytes(
		headerBits,
	)

	if len(header) < 8 ||
		string(header[:4]) != "STEG" {

		return nil, fmt.Errorf(
			"kein gültiger STEG-Payload gefunden",
		)
	}

	length := binary.LittleEndian.Uint32(
		header[4:8],
	)

	maxPayloadBytes :=
		(availableBits - 64) / 8

	if int(length) > maxPayloadBytes {
		return nil, fmt.Errorf(
			"ungültige Payload-Länge: %d",
			length,
		)
	}

	// --------------------------------------------------------
	// Nutzdaten lesen
	// --------------------------------------------------------

	payloadBits := make(
		[]byte,
		int(length)*8,
	)

	for i := 0; i < len(payloadBits); i++ {

		bitIndex := 64 + i

		posIndex := bitIndex * stride

		if posIndex >= len(positions) {
			return nil, fmt.Errorf(
				"Payload unvollständig",
			)
		}

		pos := positions[posIndex]

		payloadBits[i] =
			data[offset+pos] & 1
	}

	payload := bitsToBytes(
		payloadBits,
	)

	if len(payload) != int(length) {
		return nil, fmt.Errorf(
			"Payload unvollständig",
		)
	}

	return payload, nil
}

// ============================================================
// Universal Image
// ============================================================

func injectImageUniversal(
	img image.Image,
	payload []byte,
	seedStr string,
) (*image.RGBA, error) {

	rgba := toRGBA(img)

	bounds := rgba.Bounds()

	available :=
		bounds.Dx() *
			bounds.Dy()

	stride := strideFromSeed(
		seedStr,
	)

	if err := checkBounds(
		available,
		len(payload),
		stride,
	); err != nil {
		return nil, err
	}

	bits := bytesToBits(
		payload,
	)

	positions := securePermutation(
		available,
		seedStr,
	)

	for bitIndex := 0; bitIndex < len(bits); bitIndex++ {

		posIndex := bitIndex * stride

		if posIndex >= len(positions) {
			return nil, fmt.Errorf(
				"Payload unvollständig",
			)
		}

		pos := positions[posIndex]

		x := bounds.Min.X +
			(pos % bounds.Dx())

		y := bounds.Min.Y +
			(pos / bounds.Dx())

		idx := rgba.PixOffset(
			x,
			y,
		)

		rgba.Pix[idx] =
			(rgba.Pix[idx] & 0xFE) |
				bits[bitIndex]
	}

	return rgba, nil
}

func extractImageUniversal(
	img image.Image,
	seedStr string,
) ([]byte, error) {

	rgba := toRGBA(img)

	bounds := rgba.Bounds()

	available :=
		bounds.Dx() *
			bounds.Dy()

	stride := strideFromSeed(
		seedStr,
	)

	positions := securePermutation(
		available,
		seedStr,
	)

	availableBits := available / stride

	if availableBits < 64 {
		return nil, fmt.Errorf(
			"Bild enthält keine ausreichenden Daten",
		)
	}

	// --------------------------------------------------------
	// Header lesen
	// --------------------------------------------------------

	headerBits := make(
		[]byte,
		64,
	)

	for i := 0; i < 64; i++ {

		posIndex := i * stride

		if posIndex >= len(positions) {
			return nil, fmt.Errorf(
				"Payload unvollständig",
			)
		}

		pos := positions[posIndex]

		x := bounds.Min.X +
			(pos % bounds.Dx())

		y := bounds.Min.Y +
			(pos / bounds.Dx())

		idx := rgba.PixOffset(
			x,
			y,
		)

		headerBits[i] =
			rgba.Pix[idx] & 1
	}

	header := bitsToBytes(
		headerBits,
	)

	if len(header) < 8 ||
		string(header[:4]) != "STEG" {

		return nil, fmt.Errorf(
			"kein gültiger STEG-Payload gefunden",
		)
	}

	length := binary.LittleEndian.Uint32(
		header[4:8],
	)

	maxPayloadBytes :=
		(availableBits - 64) / 8

	if int(length) > maxPayloadBytes {
		return nil, fmt.Errorf(
			"ungültige Payload-Länge: %d",
			length,
		)
	}

	// --------------------------------------------------------
	// Nutzdaten lesen
	// --------------------------------------------------------

	payloadBits := make(
		[]byte,
		int(length)*8,
	)

	for i := 0; i < len(payloadBits); i++ {

		bitIndex := 64 + i

		posIndex := bitIndex * stride

		if posIndex >= len(positions) {
			return nil, fmt.Errorf(
				"Payload unvollständig",
			)
		}

		pos := positions[posIndex]

		x := bounds.Min.X +
			(pos % bounds.Dx())

		y := bounds.Min.Y +
			(pos / bounds.Dx())

		idx := rgba.PixOffset(
			x,
			y,
		)

		payloadBits[i] =
			rgba.Pix[idx] & 1
	}

	payload := bitsToBytes(
		payloadBits,
	)

	if len(payload) != int(length) {
		return nil, fmt.Errorf(
			"Payload unvollständig",
		)
	}

	return payload, nil
}

// ============================================================
// Datei schreiben
// ============================================================

func writeAtomic(
	path string,
	data []byte,
) error {

	tmp := path + ".tmp"

	if err := os.WriteFile(
		tmp,
		data,
		0644,
	); err != nil {
		return fmt.Errorf(
			"temporäre Datei konnte nicht geschrieben werden: %w",
			err,
		)
	}

	if err := os.Rename(
		tmp,
		path,
	); err != nil {

		_ = os.Remove(tmp)

		return fmt.Errorf(
			"atomares Ersetzen fehlgeschlagen: %w",
			err,
		)
	}

	return nil
}

// ============================================================
// steg.Inject
// ============================================================

func stegInject(
	inPath string,
	outPath string,
	dataB64 string,
	seedStr string,
) jsonValue {

	data, err := base64.StdEncoding.DecodeString(
		dataB64,
	)

	if err != nil {
		return makeArray([]jsonValue{
			makeBool(false),
			makeString(seedStr),
			makeString(
				"ungültige Base64-Daten: " + err.Error(),
			),
		})
	}

	inputPath, err := normalizePath(inPath)

	if err != nil {
		return makeArray([]jsonValue{
			makeBool(false),
			makeString(seedStr),
			makeString(err.Error()),
		})
	}

	outputPath, err := normalizePath(outPath)

	if err != nil {
		return makeArray([]jsonValue{
			makeBool(false),
			makeString(seedStr),
			makeString(err.Error()),
		})
	}

	input, err := os.ReadFile(inputPath)

	if err != nil {
		return makeArray([]jsonValue{
			makeBool(false),
			makeString(seedStr),
			makeString(
				"Eingabedatei konnte nicht gelesen werden: " +
					err.Error(),
			),
		})
	}

	payload := buildPayload(data)

	var output []byte

	if isBMP(input) {

		output = append(
			[]byte(nil),
			input...,
		)

		if err := injectCore(
			output,
			payload,
			seedStr,
		); err != nil {

			return makeArray([]jsonValue{
				makeBool(false),
				makeString(seedStr),
				makeString(err.Error()),
			})
		}

	} else {

		img, format, err := decodeImageSafe(input)

		if err != nil {
			return makeArray([]jsonValue{
				makeBool(false),
				makeString(seedStr),
				makeString(err.Error()),
			})
		}

		rgba, err := injectImageUniversal(
			img,
			payload,
			seedStr,
		)

		if err != nil {
			return makeArray([]jsonValue{
				makeBool(false),
				makeString(seedStr),
				makeString(err.Error()),
			})
		}

		switch format {

		case "png":

			tmp := &byteBuffer{}

			if err := png.Encode(
				tmp,
				rgba,
			); err != nil {

				return makeArray([]jsonValue{
					makeBool(false),
					makeString(seedStr),
					makeString(
						"PNG konnte nicht geschrieben werden: " +
							err.Error(),
					),
				})
			}

			output = tmp.Bytes()

		case "bmp":

			tmp := &byteBuffer{}

			if err := bmp.Encode(
				tmp,
				rgba,
			); err != nil {

				return makeArray([]jsonValue{
					makeBool(false),
					makeString(seedStr),
					makeString(
						"BMP konnte nicht geschrieben werden: " +
							err.Error(),
					),
				})
			}

			output = tmp.Bytes()

		default:

			return makeArray([]jsonValue{
				makeBool(false),
				makeString(seedStr),
				makeString(
					"Nicht unterstütztes Bildformat: " + format,
				),
			})
		}
	}

	if err := writeAtomic(
		outputPath,
		output,
	); err != nil {

		return makeArray([]jsonValue{
			makeBool(false),
			makeString(seedStr),
			makeString(err.Error()),
		})
	}

	return makeArray([]jsonValue{
		makeBool(true),
		makeString(seedStr),
		makeString(""),
	})
}

// ============================================================
// steg.Extract
// ============================================================

func stegExtract(
	path string,
	seedStr string,
) jsonValue {

	filePath, err := normalizePath(path)

	if err != nil {
		return makeArray([]jsonValue{
			makeBool(false),
			makeString(""),
			makeString(err.Error()),
		})
	}

	data, err := os.ReadFile(filePath)

	if err != nil {
		return makeArray([]jsonValue{
			makeBool(false),
			makeString(""),
			makeString(
				"Datei konnte nicht gelesen werden: " +
					err.Error(),
			),
		})
	}

	var payload []byte

	if isBMP(data) {

		payload, err = extractCore(
			data,
			seedStr,
		)

	} else {

		img, _, decodeErr := decodeImageSafe(data)

		if decodeErr != nil {
			return makeArray([]jsonValue{
				makeBool(false),
				makeString(""),
				makeString(decodeErr.Error()),
			})
		}

		payload, err = extractImageUniversal(
			img,
			seedStr,
		)
	}

	if err != nil {
		return makeArray([]jsonValue{
			makeBool(false),
			makeString(""),
			makeString(err.Error()),
		})
	}

	return makeArray([]jsonValue{
		makeBool(true),
		makeString(
			base64.StdEncoding.EncodeToString(
				payload,
			),
		),
		makeString(""),
	})
}

// ============================================================
// steg.GenerateSeed
// ============================================================

func stegGenerateSeed(
	password string,
	salt string,
) string {

	mac := hmac.New(
		sha256.New,
		[]byte(password),
	)

	_, _ = mac.Write(
		[]byte(salt),
	)

	return hex.EncodeToString(
		mac.Sum(nil),
	)
}

// ============================================================
// steg.GetCapacity
// ============================================================

func stegGetCapacity(
	path string,
	dataLen float64,
	seedStr string,
) jsonValue {

	filePath, err := normalizePath(path)

	if err != nil {
		return makeCapacityError(err)
	}

	data, err := os.ReadFile(filePath)

	if err != nil {
		return makeCapacityError(
			fmt.Errorf(
				"Datei konnte nicht gelesen werden: %w",
				err,
			),
		)
	}

	stride := strideFromSeed(seedStr)

	var availableUnits int

	if isBMP(data) {

		if len(data) < 14 {
			return makeCapacityError(
				fmt.Errorf(
					"ungültige BMP-Datei",
				),
			)
		}

		offset := int(
			binary.LittleEndian.Uint32(
				data[10:14],
			),
		)

		if offset < 14 || offset >= len(data) {
			return makeCapacityError(
				fmt.Errorf(
					"ungültiger BMP-Pixeloffset: %d",
					offset,
				),
			)
		}

		availableUnits =
			len(data) - offset

	} else {

		img, _, err := decodeImageSafe(data)

		if err != nil {
			return makeCapacityError(err)
		}

		bounds := img.Bounds()

		availableUnits =
			bounds.Dx() *
				bounds.Dy()
	}

	// 8 Byte Header:
	//   4 Byte Magic
	//   4 Byte Payload-Länge
	const headerBytes = 8

	maxBytes :=
		availableUnits/stride/8 -
			headerBytes

	if maxBytes < 0 {
		maxBytes = 0
	}

	nettoBytes :=
		maxBytes * 65 / 100

	requested := int(dataLen)

	if requested < 0 {
		requested = 0
	}

	errMessage := ""

	if requested > maxBytes {
		errMessage = fmt.Sprintf(
			"angeforderte Datenmenge %d Bytes überschreitet die maximale Kapazität von %d Bytes",
			requested,
			maxBytes,
		)
	}

	return makeArray([]jsonValue{
		makeBool(errMessage == ""),
		makeNum(float64(nettoBytes)),
		makeString(errMessage),
		makeNum(float64(maxBytes)),
	})
}

// ============================================================
// Byte Buffer
// ============================================================

type byteBuffer struct {
	data []byte
}

func (b *byteBuffer) Write(
	p []byte,
) (int, error) {

	b.data = append(
		b.data,
		p...,
	)

	return len(p), nil
}

func (b *byteBuffer) Bytes() []byte {
	return b.data
}

// ============================================================
// Plugin Funktionen
// ============================================================

func callFunction(
	name string,
	args []jsonValue,
) jsonValue {

	switch name {

	// --------------------------------------------------------
	// steg.Inject
	// --------------------------------------------------------

	case "Inject":

		if len(args) < 4 {
			return makeError(
				fmt.Errorf(
					"steg.Inject(inPath, outPath, dataB64, seed) benötigt 4 Argumente",
				),
			)
		}

		inPath, err := argString(args, 0)

		if err != nil {
			return makeError(err)
		}

		outPath, err := argString(args, 1)

		if err != nil {
			return makeError(err)
		}

		dataB64, err := argString(args, 2)

		if err != nil {
			return makeError(err)
		}

		seedStr, err := argString(args, 3)

		if err != nil {
			return makeError(err)
		}

		return stegInject(
			inPath,
			outPath,
			dataB64,
			seedStr,
		)

	// --------------------------------------------------------
	// steg.Extract
	// --------------------------------------------------------

	case "Extract":

		if len(args) < 2 {
			return makeError(
				fmt.Errorf(
					"steg.Extract(path, seed) benötigt 2 Argumente",
				),
			)
		}

		path, err := argString(args, 0)

		if err != nil {
			return makeError(err)
		}

		seedStr, err := argString(args, 1)

		if err != nil {
			return makeError(err)
		}

		return stegExtract(
			path,
			seedStr,
		)

	// --------------------------------------------------------
	// steg.GenerateSeed
	// --------------------------------------------------------

	case "GenerateSeed":

		if len(args) < 2 {
			return makeError(
				fmt.Errorf(
					"steg.GenerateSeed(password, salt) benötigt 2 Argumente",
				),
			)
		}

		password, err := argString(args, 0)

		if err != nil {
			return makeError(err)
		}

		salt, err := argString(args, 1)

		if err != nil {
			return makeError(err)
		}

		return makeString(
			stegGenerateSeed(
				password,
				salt,
			),
		)

	// --------------------------------------------------------
	// steg.GetCapacity
	// --------------------------------------------------------

	case "GetCapacity":

		if len(args) < 3 {
			return makeError(
				fmt.Errorf(
					"steg.GetCapacity(path, dataLen, seed) benötigt 3 Argumente",
				),
			)
		}

		path, err := argString(args, 0)

		if err != nil {
			return makeError(err)
		}

		dataLen, err := argNum(args, 1)

		if err != nil {
			return makeError(err)
		}

		seedStr, err := argString(args, 2)

		if err != nil {
			return makeError(err)
		}

		return stegGetCapacity(
			path,
			dataLen,
			seedStr,
		)

	default:

		return makeError(
			fmt.Errorf(
				"unbekannte Funktion: steg.%s",
				name,
			),
		)
	}
}

// ============================================================
// ABI: vbx_abi_version
// ============================================================

//go:wasmexport vbx_abi_version
func vbxABIVersion() uint32 {
	return abiVersion
}

// ============================================================
// ABI: vbx_describe
// ============================================================

type pluginFuncDesc struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Params      string `json:"params"`
	Description string `json:"description"`
}

var descriptionJSON []byte

func initDescription() {

	functions := []pluginFuncDesc{

		{
			Namespace:   "steg",
			Name:        "Inject",
			Params:      "inPath, outPath, dataB64, seed",
			Description: "Versteckt Base64-Daten in einem Bild.",
		},

		{
			Namespace:   "steg",
			Name:        "Extract",
			Params:      "path, seed",
			Description: "Extrahiert versteckte Daten aus einem Bild.",
		},

		{
			Namespace:   "steg",
			Name:        "GenerateSeed",
			Params:      "password, salt",
			Description: "Erzeugt einen deterministischen HMAC-SHA256-Seed.",
		},

		{
			Namespace:   "steg",
			Name:        "GetCapacity",
			Params:      "path, dataLen, seed",
			Description: "Ermittelt die verfügbare Steganografie-Kapazität eines Bildes.",
		},
	}

	descriptionJSON, _ = json.Marshal(
		functions,
	)
}

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {

	if descriptionJSON == nil {
		initDescription()
	}

	ptr := alloc(
		uint32(len(descriptionJSON)),
	)

	buf := liveBuffers[ptr]

	copy(
		buf,
		descriptionJSON,
	)

	return (uint64(ptr) << 32) |
		uint64(len(descriptionJSON))
}

// ============================================================
// ABI: vbx_call
// ============================================================

//go:wasmexport vbx_call
func vbxCall(
	namePtr uint32,
	nameLen uint32,
	argsPtr uint32,
	argsLen uint32,
) uint64 {

	nameBuf, ok := liveBuffers[namePtr]

	if !ok || nameLen > uint32(len(nameBuf)) {

		return returnJSON(
			makeError(
				fmt.Errorf(
					"ungültiger Funktionsname-Pointer",
				),
			),
		)
	}

	name := string(
		nameBuf[:nameLen],
	)

	args, err := readArgs(
		argsPtr,
		argsLen,
	)

	if err != nil {

		return returnJSON(
			makeError(err),
		)
	}

	result := callFunction(
		name,
		args,
	)

	return returnJSON(result)
}

// ------------------------------------------------------------
// Rückgabe in WASM-Speicher
// ------------------------------------------------------------

func returnJSON(
	value jsonValue,
) uint64 {

	data, err := json.Marshal(
		value,
	)

	if err != nil {
		data = []byte(
			`{"type":"error","message":"JSON encoding failed"}`,
		)
	}

	ptr := alloc(
		uint32(len(data)),
	)

	buf := liveBuffers[ptr]

	copy(
		buf,
		data,
	)

	return (uint64(ptr) << 32) |
		uint64(len(data))
}

// ============================================================
// main
// ============================================================

func main() {}
