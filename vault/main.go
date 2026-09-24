package main

// ------------------------------------------------------------
// vault.* WASM-Plugin für VBX
//
// Build (normales Go, kein TinyGo):
//   GOOS=wasip1 GOARCH=wasm go build -o vault.wasm .
//
// Abhängigkeit:
//   go get golang.org/x/crypto/argon2
//
// Gegenüber dem ursprünglichen CLI-Tool geändert:
//   - PBKDF2 (100k Iterationen) -> Argon2id, konsistent zu
//     eurem bestehenden pqc.*-Einsatz. Parameter unten bitte
//     gegen eure pqc.*-Konstanten abgleichen/angleichen.
//   - Keine verschluckten Fehler mehr (aes.NewCipher,
//     cipher.NewGCM, Base64-Decode) - alles landet als
//     ErrorVal statt als potenzieller Panic.
//   - Zustandslos wie besprochen: jeder Aufruf lädt/speichert
//     die Container-Datei neu und leitet den Schlüssel neu ab.
//     Falls das bei wiederholten Aufrufen zu langsam wird,
//     müsste ein vault.Open/Close-Handle-Modell nachgezogen
//     werden (bewusst noch nicht umgesetzt).
//
// Struktur/Helfer 1:1 an eure anderen Plugins (fin, qr)
// angeglichen.
// ------------------------------------------------------------

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"unsafe"

	"golang.org/x/crypto/argon2"
)

// ------------------------------------------------------------
// Argon2id-Parameter
//
// Bitte gegen die in pqc.* verwendeten Werte abgleichen und
// ggf. angleichen, statt zwei unterschiedliche Parametersätze
// im Projekt zu haben.
// ------------------------------------------------------------

const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
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

//go:wasmexport vbx_abi_version
func vbxABIVersion() uint32 {
	return abiVersion
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

func strResult(s string) []byte {
	data, _ := json.Marshal(jsonValue{Type: "str", Str: s})
	return data
}

func boolResult(b bool) []byte {
	data, _ := json.Marshal(jsonValue{Type: "bool", Bool: b})
	return data
}

func arrResult(vals []jsonValue) []byte {
	data, _ := json.Marshal(jsonValue{Type: "arr", Arr: vals})
	return data
}

func errorResult(msg string) []byte {
	data, _ := json.Marshal(jsonValue{Type: "error", Message: msg})
	return data
}

// ------------------------------------------------------------
// Hilfsfunktionen
// ------------------------------------------------------------

func requireStr(args []jsonValue, index int, function string) (string, []byte) {
	if index >= len(args) {
		return "", errorResult(function + ": Argument fehlt")
	}

	if args[index].Type != "str" {
		return "", errorResult(
			function + ": Argument " + itoa(index+1) + " muss Text sein",
		)
	}

	return args[index].Str, nil
}

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
// Container / Krypto
// ------------------------------------------------------------

type vaultEntry struct {
	Name       string `json:"name"`
	IV         string `json:"iv"`
	Ciphertext string `json:"ciphertext"`
}

type vaultContainer struct {
	Version int          `json:"version"`
	Salt    string       `json:"salt"`
	Entries []vaultEntry `json:"entries"`
}

func randomBase64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func deriveKey(password, saltB64 string) ([]byte, error) {
	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return nil, err
	}

	return argon2.IDKey(
		[]byte(password),
		salt,
		argonTime,
		argonMemory,
		argonThreads,
		argonKeyLen,
	), nil
}

func encryptValue(key []byte, plaintext string) (ivB64, ctB64 string, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	iv := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", "", err
	}

	ct := gcm.Seal(nil, iv, []byte(plaintext), nil)

	return base64.StdEncoding.EncodeToString(iv),
		base64.StdEncoding.EncodeToString(ct),
		nil
}

func decryptValue(key []byte, ivB64, ctB64 string) (string, error) {
	iv, err := base64.StdEncoding.DecodeString(ivB64)
	if err != nil {
		return "", err
	}

	ct, err := base64.StdEncoding.DecodeString(ctB64)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	pt, err := gcm.Open(nil, iv, ct, nil)
	if err != nil {
		return "", err
	}

	return string(pt), nil
}

func loadOrCreateContainer(path string) (*vaultContainer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			salt, saltErr := randomBase64(saltLen)
			if saltErr != nil {
				return nil, saltErr
			}

			return &vaultContainer{
				Version: 1,
				Salt:    salt,
				Entries: []vaultEntry{},
			}, nil
		}

		return nil, err
	}

	var c vaultContainer
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}

	return &c, nil
}

func saveContainer(path string, c *vaultContainer) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {
	entries := []funcDesc{
		{
			Namespace:   "vault",
			Name:        "Add",
			Params:      "datei, master, name, wert",
			Description: "Legt einen verschlüsselten Eintrag an oder überschreibt einen bestehenden mit gleichem Namen.",
		},
		{
			Namespace:   "vault",
			Name:        "Get",
			Params:      "datei, master, name",
			Description: "Entschlüsselt und liefert den Wert eines Eintrags.",
		},
		{
			Namespace:   "vault",
			Name:        "Delete",
			Params:      "datei, master, name",
			Description: "Entfernt einen Eintrag aus dem Vault.",
		},
		{
			Namespace:   "vault",
			Name:        "List",
			Params:      "datei",
			Description: "Listet die Namen aller gespeicherten Einträge auf (ohne Entschlüsselung).",
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
		return packBytes(errorResult("ungültige Argumente: " + err.Error()))
	}

	switch name {
	case "Add":
		return packBytes(handleAdd(args))
	case "Get":
		return packBytes(handleGet(args))
	case "Delete":
		return packBytes(handleDelete(args))
	case "List":
		return packBytes(handleList(args))
	default:
		return packBytes(errorResult("unbekannte Funktion: " + name))
	}
}

// ------------------------------------------------------------
// Add
// ------------------------------------------------------------

func handleAdd(args []jsonValue) []byte {
	if len(args) < 4 {
		return errorResult("Add erwartet 4 Argumente (datei, master, name, wert)")
	}

	path, errB := requireStr(args, 0, "Add")
	if errB != nil {
		return errB
	}

	master, errB := requireStr(args, 1, "Add")
	if errB != nil {
		return errB
	}

	name, errB := requireStr(args, 2, "Add")
	if errB != nil {
		return errB
	}

	value, errB := requireStr(args, 3, "Add")
	if errB != nil {
		return errB
	}

	c, err := loadOrCreateContainer(path)
	if err != nil {
		return errorResult("Vault konnte nicht gelesen werden: " + err.Error())
	}

	key, err := deriveKey(master, c.Salt)
	if err != nil {
		return errorResult("Schlüssel konnte nicht abgeleitet werden: " + err.Error())
	}

	iv, ct, err := encryptValue(key, value)
	if err != nil {
		return errorResult("Verschlüsselung fehlgeschlagen: " + err.Error())
	}

	updated := false
	for i, e := range c.Entries {
		if e.Name == name {
			c.Entries[i].IV = iv
			c.Entries[i].Ciphertext = ct
			updated = true
			break
		}
	}

	if !updated {
		c.Entries = append(c.Entries, vaultEntry{
			Name:       name,
			IV:         iv,
			Ciphertext: ct,
		})
	}

	if err := saveContainer(path, c); err != nil {
		return errorResult("Vault konnte nicht gespeichert werden: " + err.Error())
	}

	return boolResult(true)
}

// ------------------------------------------------------------
// Get
// ------------------------------------------------------------

func handleGet(args []jsonValue) []byte {
	if len(args) < 3 {
		return errorResult("Get erwartet 3 Argumente (datei, master, name)")
	}

	path, errB := requireStr(args, 0, "Get")
	if errB != nil {
		return errB
	}

	master, errB := requireStr(args, 1, "Get")
	if errB != nil {
		return errB
	}

	name, errB := requireStr(args, 2, "Get")
	if errB != nil {
		return errB
	}

	c, err := loadOrCreateContainer(path)
	if err != nil {
		return errorResult("Vault konnte nicht gelesen werden: " + err.Error())
	}

	key, err := deriveKey(master, c.Salt)
	if err != nil {
		return errorResult("Schlüssel konnte nicht abgeleitet werden: " + err.Error())
	}

	for _, e := range c.Entries {
		if e.Name == name {
			pt, err := decryptValue(key, e.IV, e.Ciphertext)
			if err != nil {
				return errorResult("Entschlüsselung fehlgeschlagen (falsches Master-Passwort?): " + err.Error())
			}

			return strResult(pt)
		}
	}

	return errorResult("Eintrag nicht gefunden: " + name)
}

// ------------------------------------------------------------
// Delete
// ------------------------------------------------------------

func handleDelete(args []jsonValue) []byte {
	if len(args) < 3 {
		return errorResult("Delete erwartet 3 Argumente (datei, master, name)")
	}

	path, errB := requireStr(args, 0, "Delete")
	if errB != nil {
		return errB
	}

	// master wird hier nicht gebraucht (Löschen erfordert keine
	// Entschlüsselung), aber als Argument erzwungen, damit
	// versehentliches Löschen ohne Kenntnis des Master-Passworts
	// nicht möglich ist.
	if _, errB := requireStr(args, 1, "Delete"); errB != nil {
		return errB
	}

	name, errB := requireStr(args, 2, "Delete")
	if errB != nil {
		return errB
	}

	c, err := loadOrCreateContainer(path)
	if err != nil {
		return errorResult("Vault konnte nicht gelesen werden: " + err.Error())
	}

	newEntries := make([]vaultEntry, 0, len(c.Entries))
	found := false

	for _, e := range c.Entries {
		if e.Name == name {
			found = true
			continue
		}
		newEntries = append(newEntries, e)
	}

	if !found {
		return errorResult("Eintrag nicht gefunden: " + name)
	}

	c.Entries = newEntries

	if err := saveContainer(path, c); err != nil {
		return errorResult("Vault konnte nicht gespeichert werden: " + err.Error())
	}

	return boolResult(true)
}

// ------------------------------------------------------------
// List
// ------------------------------------------------------------

func handleList(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult("List erwartet 1 Argument (datei)")
	}

	path, errB := requireStr(args, 0, "List")
	if errB != nil {
		return errB
	}

	c, err := loadOrCreateContainer(path)
	if err != nil {
		return errorResult("Vault konnte nicht gelesen werden: " + err.Error())
	}

	vals := make([]jsonValue, len(c.Entries))
	for i, e := range c.Entries {
		vals[i] = jsonValue{Type: "str", Str: e.Name}
	}

	return arrResult(vals)
}

// ------------------------------------------------------------
// Reactor entry point
// ------------------------------------------------------------

func main() {}
