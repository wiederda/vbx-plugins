package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	crand "crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"unicode"
	"unsafe"

	"golang.org/x/crypto/bcrypt"
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
	Map     map[string]jsonValue `json:"map,omitempty"`
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

func strResult(value string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "str",
		Str:  value,
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

func mapResult(m map[string]jsonValue) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "map",
		Map:  m,
	})

	return data
}

// arr3Result baut die [bool, str, str]-Konvention nach
// (AESEncrypt/AESDecrypt/AESDecryptFileToString).
func arr3Result(ok bool, val, msg string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "arr",
		Arr: []jsonValue{
			{Type: "bool", Bool: ok},
			{Type: "str", Str: val},
			{Type: "str", Str: msg},
		},
	})

	return data
}

// arr2Result baut die [bool, str]-Konvention nach
// (AESEncryptFile/AESDecryptFile).
func arr2Result(ok bool, msg string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "arr",
		Arr: []jsonValue{
			{Type: "bool", Bool: ok},
			{Type: "str", Str: msg},
		},
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

func getIntArg(
	args []jsonValue,
	idx int,
	defaultValue int,
) int {

	if len(args) <= idx || args[idx].Type != "num" {
		return defaultValue
	}

	return int(args[idx].Num)
}

func getBoolArg(
	args []jsonValue,
	idx int,
	defaultValue bool,
) bool {

	if len(args) <= idx || args[idx].Type != "bool" {
		return defaultValue
	}

	return args[idx].Bool
}

func getByteArrayArg(
	args []jsonValue,
	idx int,
	funcName string,
) ([]byte, error) {

	if len(args) <= idx {
		return nil, fmt.Errorf(
			"%s: Argument %d fehlt",
			funcName,
			idx+1,
		)
	}

	if args[idx].Type != "arr" {
		return nil, fmt.Errorf(
			"%s: Argument %d muss ein Array sein",
			funcName,
			idx+1,
		)
	}

	buf := make([]byte, len(args[idx].Arr))

	for i, v := range args[idx].Arr {
		if v.Type != "num" {
			return nil, fmt.Errorf(
				"%s: Array enthält Nicht-Zahlen-Werte",
				funcName,
			)
		}
		buf[i] = byte(int(v.Num))
	}

	return buf, nil
}

func absPathStrict(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf(
			"pfad '%s' konnte nicht aufgelöst werden: %w",
			p,
			err,
		)
	}
	return abs, nil
}

func cryptoRandInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := crand.Int(crand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

func shuffleBytes(b []byte) {
	for i := len(b) - 1; i > 0; i-- {
		j := cryptoRandInt(i + 1)
		b[i], b[j] = b[j], b[i]
	}
}

func shuffleInts(s []int) {
	for i := len(s) - 1; i > 0; i-- {
		j := cryptoRandInt(i + 1)
		s[i], s[j] = s[j], s[i]
	}
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {

	entries := []funcDesc{
		{
			Namespace:   "crypt",
			Name:        "GUID",
			Params:      "-",
			Description: "Generiert eine eindeutige GUID.",
		},
		{
			Namespace:   "crypt",
			Name:        "BytesToString",
			Params:      "byteArray",
			Description: "Wandelt ein Byte-Array (0-255 Werte) in einen UTF-8-String um. Für kurzzeitige Verwendung gedacht.",
		},
		{
			Namespace:   "crypt",
			Name:        "RNGCryptoProvider",
			Params:      "length",
			Description: "Erzeugt kryptografisch sichere Zufallsbytes und gibt sie als Hex-String zurück.",
		},
		{
			Namespace:   "crypt",
			Name:        "RandomString",
			Params:      "length",
			Description: "Erzeugt eine zufällige alphanumerische Zeichenfolge (min. 10 Zeichen).",
		},
		{
			Namespace:   "crypt",
			Name:        "RandomPassword",
			Params:      "len, [num], [low], [up], [spec], [head]",
			Description: "Generiert ein komplexes Zufallspasswort.",
		},
		{
			Namespace:   "crypt",
			Name:        "AESEncrypt",
			Params:      "text, pass",
			Description: "Verschlüsselt Text mit AES-256-GCM. Rückgabe: [OK, CipherBase64, Msg]",
		},
		{
			Namespace:   "crypt",
			Name:        "AESDecrypt",
			Params:      "cipher, pass",
			Description: "Entschlüsselt AES-GCM Daten. Rückgabe: [OK, Plaintext, Msg]",
		},
		{
			Namespace:   "crypt",
			Name:        "AESEncryptFile",
			Params:      "sourcePath, destPath, pass [, deleteSource]",
			Description: "Verschlüsselt eine Datei mit AES-256-GCM. Rückgabe: [OK, Msg]",
		},
		{
			Namespace:   "crypt",
			Name:        "AESDecryptFileToString",
			Params:      "sourcePath, pass",
			Description: "Entschlüsselt eine Datei direkt im Speicher, ohne Klartext zu schreiben. Rückgabe: [OK, Content, Msg]",
		},
		{
			Namespace:   "crypt",
			Name:        "AESDecryptFile",
			Params:      "sourcePath, destPath, pass [, deleteSource]",
			Description: "Entschlüsselt eine Datei und speichert sie unter destPath. Rückgabe: [OK, Msg]",
		},
		{
			Namespace:   "crypt",
			Name:        "CheckPassword",
			Params:      "password [, minLength] [, maxLength] [, requireUpper] [, requireLower] [, requireDigit] [, requireSpecial]",
			Description: "Prüft ein Passwort gegen konfigurierbare Regeln. Gibt eine Map mit Detail-Ergebnissen zurück.",
		},
		{
			Namespace:   "crypt",
			Name:        "HMAC",
			Params:      "s, key, [algo]",
			Description: "Erzeugt einen HMAC. algo: sha256 (Standard), sha512, sha1, md5.",
		},
		{
			Namespace:   "crypt",
			Name:        "Bcrypt",
			Params:      "pass, [cost]",
			Description: "Erzeugt einen sicheren Bcrypt-Hash eines Passworts. cost: 4-31, Standard 10.",
		},
		{
			Namespace:   "crypt",
			Name:        "BcryptVerify",
			Params:      "pass, hash",
			Description: "Prüft ob ein Passwort zu einem Bcrypt-Hash passt.",
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

	case "GUID":
		return packBytes(handleGUID(args))

	case "BytesToString":
		return packBytes(handleBytesToString(args))

	case "RNGCryptoProvider":
		return packBytes(handleRNGCryptoProvider(args))

	case "RandomString":
		return packBytes(handleRandomString(args))

	case "RandomPassword":
		return packBytes(handleRandomPassword(args))

	case "AESEncrypt":
		return packBytes(handleAESEncrypt(args))

	case "AESDecrypt":
		return packBytes(handleAESDecrypt(args))

	case "AESEncryptFile":
		return packBytes(handleAESEncryptFile(args))

	case "AESDecryptFileToString":
		return packBytes(handleAESDecryptFileToString(args))

	case "AESDecryptFile":
		return packBytes(handleAESDecryptFile(args))

	case "CheckPassword":
		return packBytes(handleCheckPassword(args))

	case "HMAC":
		return packBytes(handleHMAC(args))

	case "Bcrypt":
		return packBytes(handleBcrypt(args))

	case "BcryptVerify":
		return packBytes(handleBcryptVerify(args))

	default:
		return packBytes(
			errorResult(
				"unbekannte Funktion: " + name,
			),
		)
	}
}

// ------------------------------------------------------------
// GUID / BytesToString / RNG
// ------------------------------------------------------------

func handleGUID(args []jsonValue) []byte {

	b := make([]byte, 16)

	if _, err := crand.Read(b); err != nil {
		return arr3Result(false, "", "RNG Fehler: "+err.Error())
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	guid := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])

	return arr3Result(true, guid, "")
}

func handleBytesToString(args []jsonValue) []byte {

	buf, err := getByteArrayArg(args, 0, "crypt.BytesToString")
	if err != nil {
		return errorResult(err.Error())
	}

	return strResult(string(buf))
}

func handleRNGCryptoProvider(args []jsonValue) []byte {

	length := getIntArg(args, 0, 16)

	b := make([]byte, length)
	if _, err := crand.Read(b); err != nil {
		return arr3Result(false, "", "RNG Fehler: "+err.Error())
	}

	return arr3Result(true, fmt.Sprintf("%x", b), "")
}

// ------------------------------------------------------------
// RandomString / RandomPassword
// ------------------------------------------------------------

func handleRandomString(args []jsonValue) []byte {

	length := getIntArg(args, 0, 10)
	if length < 10 {
		length = 10
	}

	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, length)

	max := big.NewInt(int64(len(chars)))

	for i := 0; i < length; i++ {
		nBig, err := crand.Int(crand.Reader, max)
		if err != nil {
			return errorResult("RNG Fehler: " + err.Error())
		}
		buf[i] = chars[nBig.Int64()]
	}

	return strResult(string(buf))
}

func handleRandomPassword(args []jsonValue) []byte {

	length := getIntArg(args, 0, 12)
	numbers := getBoolArg(args, 1, false)
	lower := getBoolArg(args, 2, false)
	upper := getBoolArg(args, 3, false)
	specials := getBoolArg(args, 4, false)
	firstLetter := getBoolArg(args, 5, true)

	if length < 10 {
		length = 10
	}

	if !(numbers || lower || upper || specials) {
		lower = true
	}

	lowerChars := "abcdefghijklmnopqrstuvwxyz"
	upperChars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	numberChars := "0123456789"
	specialChars := "+-/*#,;.:-_^!()[]{}=?<>@"
	letters := lowerChars + upperChars

	if firstLetter && !lower && !upper {
		lower = true
	}

	charset := ""
	mustInclude := []byte{}

	if numbers {
		charset += numberChars
		mustInclude = append(mustInclude, numberChars[cryptoRandInt(len(numberChars))])
	}
	if lower {
		charset += lowerChars
		mustInclude = append(mustInclude, lowerChars[cryptoRandInt(len(lowerChars))])
	}
	if upper {
		charset += upperChars
		mustInclude = append(mustInclude, upperChars[cryptoRandInt(len(upperChars))])
	}
	if specials {
		charset += specialChars
		mustInclude = append(mustInclude, specialChars[cryptoRandInt(len(specialChars))])
	}

	pass := make([]byte, length)
	for i := range pass {
		pass[i] = charset[cryptoRandInt(len(charset))]
	}

	indices := make([]int, length)
	for i := 0; i < length; i++ {
		indices[i] = i
	}
	shuffleInts(indices)

	for i, c := range mustInclude {
		pass[indices[i]] = c
	}

	if firstLetter {
		pass[0] = letters[cryptoRandInt(len(letters))]
	}

	shuffleBytes(pass)

	return strResult(string(pass))
}

// ------------------------------------------------------------
// AES-GCM
// ------------------------------------------------------------

func handleAESEncrypt(args []jsonValue) []byte {

	if len(args) < 2 {
		return arr3Result(false, "", "AESEncrypt benötigt Text und Passwort")
	}

	text, _ := getStringArg(args, 0, "crypt.AESEncrypt", false)
	pass, _ := getStringArg(args, 1, "crypt.AESEncrypt", false)

	key := sha256.Sum256([]byte(pass))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return arr3Result(false, "", "Cipher Fehler: "+err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return arr3Result(false, "", "GCM Fehler: "+err.Error())
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return arr3Result(false, "", "Nonce Fehler: "+err.Error())
	}

	data := gcm.Seal(nonce, nonce, []byte(text), nil)
	encoded := base64.StdEncoding.EncodeToString(data)

	return arr3Result(true, encoded, "")
}

func handleAESDecrypt(args []jsonValue) []byte {

	if len(args) < 2 {
		return arr3Result(false, "", "AESDecrypt benötigt Cipher und Passwort")
	}

	cipherText, _ := getStringArg(args, 0, "crypt.AESDecrypt", false)
	pass, _ := getStringArg(args, 1, "crypt.AESDecrypt", false)

	raw, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return arr3Result(false, "", "Ungültiges Base64")
	}

	key := sha256.Sum256([]byte(pass))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return arr3Result(false, "", "Cipher Fehler: "+err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return arr3Result(false, "", "GCM Fehler: "+err.Error())
	}

	nsz := gcm.NonceSize()
	if len(raw) < nsz {
		return arr3Result(false, "", "Daten zu kurz")
	}

	nonce := raw[:nsz]
	cipherData := raw[nsz:]

	plain, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return arr3Result(false, "", "Falsches Passwort oder Daten korrupt")
	}

	return arr3Result(true, string(plain), "")
}

func handleAESEncryptFile(args []jsonValue) []byte {

	if len(args) < 3 {
		return arr2Result(false, "AESEncryptFile benötigt sourcePath, destPath und Passwort")
	}

	srcRaw, _ := getStringArg(args, 0, "crypt.AESEncryptFile", false)
	destRaw, _ := getStringArg(args, 1, "crypt.AESEncryptFile", false)
	pass, _ := getStringArg(args, 2, "crypt.AESEncryptFile", false)
	deleteSource := getBoolArg(args, 3, false)

	srcPath, err := absPathStrict(srcRaw)
	if err != nil {
		return arr2Result(false, "Pfad-Fehler (sourcePath): "+err.Error())
	}

	destPath, err := absPathStrict(destRaw)
	if err != nil {
		return arr2Result(false, "Pfad-Fehler (destPath): "+err.Error())
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return arr2Result(false, "Datei-Lesefehler: "+err.Error())
	}

	key := sha256.Sum256([]byte(pass))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return arr2Result(false, "Cipher Fehler: "+err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return arr2Result(false, "GCM Fehler: "+err.Error())
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return arr2Result(false, "Nonce Fehler: "+err.Error())
	}

	encrypted := gcm.Seal(nonce, nonce, data, nil)

	tmpPath := destPath + ".tmp"
	if err := os.WriteFile(tmpPath, encrypted, 0600); err != nil {
		return arr2Result(false, "Schreibfehler: "+err.Error())
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return arr2Result(false, "Rename-Fehler: "+err.Error())
	}

	if deleteSource {
		if err := os.Remove(srcPath); err != nil {
			return arr2Result(true, "OK, aber Original konnte nicht gelöscht werden: "+err.Error())
		}
	}

	return arr2Result(true, "OK")
}

func handleAESDecryptFileToString(args []jsonValue) []byte {

	if len(args) < 2 {
		return arr3Result(false, "", "AESDecryptFileToString benötigt sourcePath und Passwort")
	}

	srcRaw, _ := getStringArg(args, 0, "crypt.AESDecryptFileToString", false)
	pass, _ := getStringArg(args, 1, "crypt.AESDecryptFileToString", false)

	srcPath, err := absPathStrict(srcRaw)
	if err != nil {
		return arr3Result(false, "", "Pfad-Fehler: "+err.Error())
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return arr3Result(false, "", "Datei-Lesefehler: "+err.Error())
	}

	key := sha256.Sum256([]byte(pass))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return arr3Result(false, "", "Cipher Fehler: "+err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return arr3Result(false, "", "GCM Fehler: "+err.Error())
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return arr3Result(false, "", "Datei ist zu kurz oder beschädigt (kein gültiger Nonce-Header)")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return arr3Result(false, "", "Entschlüsselung fehlgeschlagen (falsches Passwort oder Datei beschädigt): "+err.Error())
	}

	content := string(plain)

	// Plugin-lokale Kopie nullen - schützt nicht die Kopien, die beim
	// JSON-Marshal/Unmarshal über die ABI-Grenze entstehen (siehe
	// Hinweis unten), reduziert aber die Zeit, die der Klartext im
	// Plugin-eigenen Speicher unverschlüsselt herumliegt.
	for i := range plain {
		plain[i] = 0
	}

	return arr3Result(true, content, "OK")
}

func handleAESDecryptFile(args []jsonValue) []byte {

	if len(args) < 3 {
		return arr2Result(false, "AESDecryptFile benötigt sourcePath, destPath und Passwort")
	}

	srcRaw, _ := getStringArg(args, 0, "crypt.AESDecryptFile", false)
	destRaw, _ := getStringArg(args, 1, "crypt.AESDecryptFile", false)
	pass, _ := getStringArg(args, 2, "crypt.AESDecryptFile", false)
	deleteSource := getBoolArg(args, 3, false)

	srcPath, err := absPathStrict(srcRaw)
	if err != nil {
		return arr2Result(false, "Pfad-Fehler (sourcePath): "+err.Error())
	}

	destPath, err := absPathStrict(destRaw)
	if err != nil {
		return arr2Result(false, "Pfad-Fehler (destPath): "+err.Error())
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return arr2Result(false, "Datei-Lesefehler: "+err.Error())
	}

	key := sha256.Sum256([]byte(pass))

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return arr2Result(false, "Cipher Fehler: "+err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return arr2Result(false, "GCM Fehler: "+err.Error())
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return arr2Result(false, "Datei ist zu kurz oder beschädigt (kein gültiger Nonce-Header)")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return arr2Result(false, "Entschlüsselung fehlgeschlagen (falsches Passwort oder Datei beschädigt): "+err.Error())
	}
	defer func() {
		for i := range plain {
			plain[i] = 0
		}
	}()

	tmpPath := destPath + ".tmp"
	if err := os.WriteFile(tmpPath, plain, 0600); err != nil {
		return arr2Result(false, "Schreibfehler: "+err.Error())
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return arr2Result(false, "Rename-Fehler: "+err.Error())
	}

	if deleteSource {
		if err := os.Remove(srcPath); err != nil {
			return arr2Result(true, "OK, aber verschlüsseltes Original konnte nicht gelöscht werden: "+err.Error())
		}
	}

	return arr2Result(true, "OK")
}

// ------------------------------------------------------------
// CheckPassword
// ------------------------------------------------------------

func handleCheckPassword(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult("crypt.CheckPassword erwartet mindestens 1 Argument (password)")
	}

	pw, err := getStringArg(args, 0, "crypt.CheckPassword", true)
	if err != nil {
		return errorResult(err.Error())
	}

	minLength := getIntArg(args, 1, 8)
	maxLength := getIntArg(args, 2, 128)
	requireUpper := getBoolArg(args, 3, true)
	requireLower := getBoolArg(args, 4, true)
	requireDigit := getBoolArg(args, 5, true)
	requireSpecial := getBoolArg(args, 6, true)

	runeCount := len([]rune(pw))
	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	var errs []jsonValue

	if runeCount < minLength {
		errs = append(errs, jsonValue{Type: "str", Str: fmt.Sprintf(
			"Passwort ist zu kurz (mindestens %d Zeichen erforderlich, hat %d)", minLength, runeCount)})
	}
	if runeCount > maxLength {
		errs = append(errs, jsonValue{Type: "str", Str: fmt.Sprintf(
			"Passwort ist zu lang (maximal %d Zeichen erlaubt, hat %d)", maxLength, runeCount)})
	}
	if requireUpper && !hasUpper {
		errs = append(errs, jsonValue{Type: "str", Str: "Mindestens ein Großbuchstabe erforderlich"})
	}
	if requireLower && !hasLower {
		errs = append(errs, jsonValue{Type: "str", Str: "Mindestens ein Kleinbuchstabe erforderlich"})
	}
	if requireDigit && !hasDigit {
		errs = append(errs, jsonValue{Type: "str", Str: "Mindestens eine Ziffer erforderlich"})
	}
	if requireSpecial && !hasSpecial {
		errs = append(errs, jsonValue{Type: "str", Str: "Mindestens ein Sonderzeichen erforderlich"})
	}

	result := map[string]jsonValue{
		"Valid":      {Type: "bool", Bool: len(errs) == 0},
		"Length":     {Type: "num", Num: float64(runeCount)},
		"HasUpper":   {Type: "bool", Bool: hasUpper},
		"HasLower":   {Type: "bool", Bool: hasLower},
		"HasDigit":   {Type: "bool", Bool: hasDigit},
		"HasSpecial": {Type: "bool", Bool: hasSpecial},
		"Errors":     {Type: "arr", Arr: errs},
	}

	return mapResult(result)
}

// ------------------------------------------------------------
// HMAC / Bcrypt
// ------------------------------------------------------------

func handleHMAC(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("crypt.HMAC: s und key benötigt")
	}

	data, _ := getStringArg(args, 0, "crypt.HMAC", false)
	key, _ := getStringArg(args, 1, "crypt.HMAC", false)
	algo, _ := getStringArg(args, 2, "crypt.HMAC", false)

	if algo == "" {
		algo = "sha256"
	}

	var mac []byte

	switch algo {
	case "sha256":
		h := hmac.New(sha256.New, []byte(key))
		h.Write([]byte(data))
		mac = h.Sum(nil)
	case "sha512":
		h := hmac.New(sha512.New, []byte(key))
		h.Write([]byte(data))
		mac = h.Sum(nil)
	case "sha1":
		h := hmac.New(sha1.New, []byte(key))
		h.Write([]byte(data))
		mac = h.Sum(nil)
	case "md5":
		h := hmac.New(md5.New, []byte(key))
		h.Write([]byte(data))
		mac = h.Sum(nil)
	default:
		return errorResult("crypt.HMAC: Unbekannter Algorithmus '" + algo + "'. Unterstützt: sha256, sha512, sha1, md5")
	}

	return strResult(hex.EncodeToString(mac))
}

func handleBcrypt(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult("crypt.Bcrypt: Passwort fehlt")
	}

	pass, _ := getStringArg(args, 0, "crypt.Bcrypt", false)
	cost := getIntArg(args, 1, bcrypt.DefaultCost)

	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = bcrypt.DefaultCost
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(pass), cost)
	if err != nil {
		return errorResult("crypt.Bcrypt: " + err.Error())
	}

	return strResult(string(hashed))
}

func handleBcryptVerify(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("crypt.BcryptVerify: pass und hash benötigt")
	}

	pass, _ := getStringArg(args, 0, "crypt.BcryptVerify", false)
	hash, _ := getStringArg(args, 1, "crypt.BcryptVerify", false)

	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(pass))

	return boolResult(err == nil)
}

// ------------------------------------------------------------

func main() {}
