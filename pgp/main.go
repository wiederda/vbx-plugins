package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
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
	Type    string      `json:"type"`
	Str     string      `json:"str,omitempty"`
	Bool    bool        `json:"bool,omitempty"`
	Arr     []jsonValue `json:"arr,omitempty"`
	Message string      `json:"message,omitempty"`
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

// errArrResult / okArrResult bilden die native [bool, string, string]-
// Konvention von pgp.Sign/SignFile/Verify/VerifyFile 1:1 nach.
func errArrResult(msg string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "arr",
		Arr: []jsonValue{
			{Type: "bool", Bool: false},
			{Type: "str", Str: ""},
			{Type: "str", Str: msg},
		},
	})

	return data
}

func okArrResult(payload string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "arr",
		Arr: []jsonValue{
			{Type: "bool", Bool: true},
			{Type: "str", Str: payload},
			{Type: "str", Str: ""},
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

// ------------------------------------------------------------
// PGP-Logik (1:1 aus stdlib_pgp.go übernommen)
// ------------------------------------------------------------

func decryptEntity(entity *openpgp.Entity, pass string) error {
	if entity.PrivateKey == nil {
		return fmt.Errorf("entity hat keinen privaten Schlüssel")
	}

	if entity.PrivateKey.Encrypted {
		if pass == "" {
			return fmt.Errorf("schlüssel ist verschlüsselt, aber kein Passwort angegeben")
		}
		if err := entity.PrivateKey.Decrypt([]byte(pass)); err != nil {
			return fmt.Errorf("falsches Passwort für Hauptschlüssel")
		}
	}

	for i, sub := range entity.Subkeys {
		if sub.PrivateKey != nil && sub.PrivateKey.Encrypted {
			if err := entity.Subkeys[i].PrivateKey.Decrypt([]byte(pass)); err != nil {
				fmt.Printf("Warnung: Subkey %d konnte nicht entschlüsselt werden: %v\n", i, err)
			}
		}
	}

	return nil
}

func encryptEntity(entity *openpgp.Entity, pass string) error {
	if pass == "" {
		return nil
	}

	if err := entity.PrivateKey.Encrypt([]byte(pass)); err != nil {
		return fmt.Errorf("fehler beim Verschlüsseln des Hauptschlüssels: %w", err)
	}

	for i, sub := range entity.Subkeys {
		if sub.PrivateKey != nil {
			if err := entity.Subkeys[i].PrivateKey.Encrypt([]byte(pass)); err != nil {
				return fmt.Errorf("fehler beim Verschlüsseln von Subkey %d: %w", i, err)
			}
		}
	}

	return nil
}

func parseArmoredKey(input string) (openpgp.EntityList, error) {
	var r io.Reader

	if strings.Contains(input, "-----BEGIN PGP") {
		r = strings.NewReader(input)
	} else {
		absP, err := absPathStrict(input)
		if err != nil {
			return nil, fmt.Errorf("ungültiger Pfad: %s", input)
		}
		data, err := os.ReadFile(absP)
		if err != nil {
			return nil, fmt.Errorf("datei nicht lesbar: %w", err)
		}
		r = bytes.NewReader(data)
	}

	entities, err := openpgp.ReadArmoredKeyRing(r)
	if err != nil {
		return nil, fmt.Errorf("ungültiges PGP-Key-Format: %w", err)
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("keine Schlüssel in der Eingabe gefunden")
	}

	return entities, nil
}

func serializePrivateKey(entity *openpgp.Entity) (string, error) {
	buf := new(bytes.Buffer)
	w, err := armor.Encode(buf, openpgp.PrivateKeyType, nil)
	if err != nil {
		return "", fmt.Errorf("armor-encoder konnte nicht erstellt werden: %w", err)
	}

	if err := entity.SerializePrivate(w, nil); err != nil {
		w.Close()
		return "", fmt.Errorf("serialisierung fehlgeschlagen: %w", err)
	}
	w.Close()

	return buf.String(), nil
}

func serializePublicKey(entity *openpgp.Entity) (string, error) {
	var buf strings.Builder
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		return "", fmt.Errorf("armor-encoder konnte nicht erstellt werden: %w", err)
	}

	if err := entity.Serialize(w); err != nil {
		w.Close()
		return "", fmt.Errorf("serialisierung fehlgeschlagen: %w", err)
	}
	w.Close()

	return buf.String(), nil
}

func writeKeyFile(path, armorType string, entity *openpgp.Entity) error {
	tmpPath := path + ".tmp"

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("datei konnte nicht erstellt werden: %w", err)
	}

	w, err := armor.Encode(f, armorType, nil)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("armor-encoder fehlgeschlagen: %w", err)
	}

	var writeErr error
	if armorType == openpgp.PrivateKeyType {
		writeErr = entity.SerializePrivate(w, nil)
	} else {
		writeErr = entity.Serialize(w)
	}

	w.Close()
	f.Close()

	if writeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("serialisierung fehlgeschlagen: %w", writeErr)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("umbenennen fehlgeschlagen: %w", err)
	}

	return nil
}

func classifyVerifyError(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "signature invalid"):
		return "Inhalt wurde verändert oder Signatur passt nicht zum Text"
	case strings.Contains(s, "expired"):
		return "Signatur oder Schlüssel ist abgelaufen"
	case strings.Contains(s, "no matching keys"):
		return "Signatur wurde mit einem anderen Schlüssel erstellt"
	case strings.Contains(s, "unsupported"):
		return "Nicht unterstützter kryptografischer Algorithmus"
	default:
		return "Verifikation fehlgeschlagen: " + s
	}
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {

	entries := []funcDesc{
		{
			Namespace:   "pgp",
			Name:        "SetupIdentity",
			Params:      "folder, name, email, [password]",
			Description: "Erstellt ein neues PGP-Schlüsselpaar und speichert es im angegebenen Ordner.",
		},
		{
			Namespace:   "pgp",
			Name:        "Encrypt",
			Params:      "msg, pubKeyPath",
			Description: "Verschlüsselt eine Nachricht mit einem PGP-Public-Key.",
		},
		{
			Namespace:   "pgp",
			Name:        "Decrypt",
			Params:      "cipher, keyFolder, [password]",
			Description: "Entschlüsselt eine PGP-Nachricht mit dem privaten Schlüssel aus einem Ordner.",
		},
		{
			Namespace:   "pgp",
			Name:        "Sign",
			Params:      "msg, privKey, [password]",
			Description: "Erzeugt eine abgetrennte PGP-Signatur über eine Textnachricht.",
		},
		{
			Namespace:   "pgp",
			Name:        "SignFile",
			Params:      "filePath, privKeyArmor, [password]",
			Description: "Erzeugt eine abgetrennte PGP-Signatur über eine Datei.",
		},
		{
			Namespace:   "pgp",
			Name:        "Verify",
			Params:      "msg, sigArmor, pubKey",
			Description: "Verifiziert einen Text gegen eine abgetrennte PGP-Signatur.",
		},
		{
			Namespace:   "pgp",
			Name:        "VerifyFile",
			Params:      "filePath, sigArmor, pubKeyArmor",
			Description: "Verifiziert eine Datei gegen eine abgetrennte PGP-Signatur.",
		},
		{
			Namespace:   "pgp",
			Name:        "ChangePassword",
			Params:      "keyFolder, [oldPass], [newPass]",
			Description: "Ändert das Passwort eines PGP-Schlüssels. Leer lassen zum Entfernen.",
		},
		{
			Namespace:   "pgp",
			Name:        "ExportKey",
			Params:      "privKeyArmor, path, [password]",
			Description: "Exportiert einen PGP-Schlüssel in eine Datei, optional mit neuem Passwort.",
		},
		{
			Namespace:   "pgp",
			Name:        "ImportKey",
			Params:      "path, [password]",
			Description: "Lädt einen PGP-Schlüssel aus einer Datei und gibt ihn entschlüsselt zurück.",
		},
		{
			Namespace:   "pgp",
			Name:        "GetPublicKey",
			Params:      "pathOrKey",
			Description: "Extrahiert den öffentlichen Schlüssel aus einem Pfad oder einem Armor-String.",
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

	case "SetupIdentity":
		return packBytes(handleSetupIdentity(args))

	case "Encrypt":
		return packBytes(handleEncrypt(args))

	case "Decrypt":
		return packBytes(handleDecrypt(args))

	case "Sign":
		return packBytes(handleSign(args))

	case "SignFile":
		return packBytes(handleSignFile(args))

	case "Verify":
		return packBytes(handleVerify(args))

	case "VerifyFile":
		return packBytes(handleVerifyFile(args))

	case "ChangePassword":
		return packBytes(handleChangePassword(args))

	case "ExportKey":
		return packBytes(handleExportKey(args))

	case "ImportKey":
		return packBytes(handleImportKey(args))

	case "GetPublicKey":
		return packBytes(handleGetPublicKey(args))

	default:
		return packBytes(
			errorResult(
				"unbekannte Funktion: " + name,
			),
		)
	}
}

// ------------------------------------------------------------
// SetupIdentity
// ------------------------------------------------------------

func handleSetupIdentity(args []jsonValue) []byte {

	if len(args) < 3 {
		return errorResult(
			"Parameter fehlen: pgp.SetupIdentity(folder, name, email, [password])",
		)
	}

	folder, err := getStringArg(args, 0, "pgp.SetupIdentity", true)
	if err != nil {
		return errorResult(err.Error())
	}

	name, err := getStringArg(args, 1, "pgp.SetupIdentity", true)
	if err != nil {
		return errorResult(err.Error())
	}

	email, err := getStringArg(args, 2, "pgp.SetupIdentity", true)
	if err != nil {
		return errorResult(err.Error())
	}

	pass, err := getStringArg(args, 3, "pgp.SetupIdentity", false)
	if err != nil {
		return errorResult(err.Error())
	}

	absFolder, err := absPathStrict(folder)
	if err != nil {
		return errorResult(
			fmt.Sprintf("ungültiger Pfad '%s'", folder),
		)
	}

	if email == "" {
		email = name + "@vbx.local"
	}

	entity, err := openpgp.NewEntity(name, "vbx-generated", email, nil)
	if err != nil {
		return errorResult("Schlüsselgenerierung fehlgeschlagen: " + err.Error())
	}

	if err := encryptEntity(entity, pass); err != nil {
		return errorResult(err.Error())
	}
	if pass != "" {
		fmt.Println("PGP-Schlüssel wurde mit Passwort geschützt.")
	} else {
		fmt.Println("⚠️  WARNUNG: PGP-Privatschlüssel wurde OHNE Passwort erstellt!")
	}

	if err := os.MkdirAll(absFolder, 0700); err != nil {
		return errorResult("Ordner konnte nicht erstellt werden: " + err.Error())
	}

	pubPath := filepath.Join(absFolder, "id_pgp.pub")
	if err := writeKeyFile(pubPath, openpgp.PublicKeyType, entity); err != nil {
		return errorResult("Public Key: " + err.Error())
	}

	privPath := filepath.Join(absFolder, "id_pgp")
	if err := writeKeyFile(privPath, openpgp.PrivateKeyType, entity); err != nil {
		os.Remove(pubPath)
		return errorResult("Private Key: " + err.Error())
	}

	return strResult("OK")
}

// ------------------------------------------------------------
// Encrypt / Decrypt
// ------------------------------------------------------------

func handleEncrypt(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("Parameter fehlen: pgp.Encrypt(msg, pubKeyPath)")
	}

	msg, err := getStringArg(args, 0, "pgp.Encrypt", true)
	if err != nil {
		return errorResult(err.Error())
	}

	pubKeyPath, err := getStringArg(args, 1, "pgp.Encrypt", true)
	if err != nil {
		return errorResult(err.Error())
	}

	entityList, err := parseArmoredKey(pubKeyPath)
	if err != nil {
		return errorResult("Public Key: " + err.Error())
	}

	buf := new(bytes.Buffer)
	w, err := armor.Encode(buf, "PGP MESSAGE", nil)
	if err != nil {
		return errorResult("Armor-Encoder fehlgeschlagen: " + err.Error())
	}

	pw, err := openpgp.Encrypt(w, entityList, nil, nil, nil)
	if err != nil {
		w.Close()
		return errorResult("Verschlüsselung fehlgeschlagen: " + err.Error())
	}

	if _, err := pw.Write([]byte(msg)); err != nil {
		pw.Close()
		w.Close()
		return errorResult("Schreiben fehlgeschlagen: " + err.Error())
	}

	pw.Close()
	w.Close()

	return strResult(buf.String())
}

func handleDecrypt(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("Parameter fehlen: pgp.Decrypt(cipher, keyFolder, [password])")
	}

	cipherText, err := getStringArg(args, 0, "pgp.Decrypt", true)
	if err != nil {
		return errorResult(err.Error())
	}

	folder, err := getStringArg(args, 1, "pgp.Decrypt", true)
	if err != nil {
		return errorResult(err.Error())
	}

	pass, err := getStringArg(args, 2, "pgp.Decrypt", false)
	if err != nil {
		return errorResult(err.Error())
	}

	absFolder, err := absPathStrict(folder)
	if err != nil {
		return errorResult(fmt.Sprintf("ungültiger Pfad '%s'", folder))
	}

	keyPath := filepath.Join(absFolder, "id_pgp")
	entityList, err := parseArmoredKey(keyPath)
	if err != nil {
		return errorResult("Privater Schlüssel: " + err.Error())
	}

	for _, entity := range entityList {
		if err := decryptEntity(entity, pass); err != nil {
			return errorResult(err.Error())
		}
	}

	dec, err := armor.Decode(strings.NewReader(cipherText))
	if err != nil {
		return errorResult("Ungültiges Cipher-Format: " + err.Error())
	}

	md, err := openpgp.ReadMessage(dec.Body, entityList, nil, nil)
	if err != nil {
		return errorResult("Entschlüsselung fehlgeschlagen: " + err.Error())
	}

	content, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		return errorResult("Lesen des entschlüsselten Inhalts fehlgeschlagen: " + err.Error())
	}

	return strResult(string(content))
}

// ------------------------------------------------------------
// Sign / SignFile
// ------------------------------------------------------------

func handleSign(args []jsonValue) []byte {

	if len(args) < 2 {
		return errArrResult("Parameter fehlen: pgp.Sign(msg, privKey, [password])")
	}

	msg, err := getStringArg(args, 0, "pgp.Sign", true)
	if err != nil {
		return errArrResult(err.Error())
	}

	cleanMsg := strings.TrimSpace(msg)

	privKey, err := getStringArg(args, 1, "pgp.Sign", true)
	if err != nil {
		return errArrResult(err.Error())
	}

	pass, err := getStringArg(args, 2, "pgp.Sign", false)
	if err != nil {
		return errArrResult(err.Error())
	}

	entityList, err := parseArmoredKey(privKey)
	if err != nil {
		return errArrResult("Privater Schlüssel: " + err.Error())
	}

	if err := decryptEntity(entityList[0], pass); err != nil {
		return errArrResult(err.Error())
	}

	buf := new(bytes.Buffer)
	w, err := armor.Encode(buf, openpgp.SignatureType, nil)
	if err != nil {
		return errArrResult("Armor-Encoder fehlgeschlagen: " + err.Error())
	}

	if err := openpgp.DetachSign(w, entityList[0], strings.NewReader(cleanMsg), nil); err != nil {
		w.Close()
		return errArrResult("Signierfehler: " + err.Error())
	}
	w.Close()

	return okArrResult(buf.String())
}

func handleSignFile(args []jsonValue) []byte {

	if len(args) < 2 {
		return errArrResult("Parameter fehlen: pgp.SignFile(filePath, privKeyArmor, [password])")
	}

	filePath, err := getStringArg(args, 0, "pgp.SignFile", true)
	if err != nil {
		return errArrResult(err.Error())
	}

	absPath, err := absPathStrict(filePath)
	if err != nil {
		return errArrResult(fmt.Sprintf("ungültiger Pfad '%s'", filePath))
	}

	privKeyArmor, err := getStringArg(args, 1, "pgp.SignFile", true)
	if err != nil {
		return errArrResult(err.Error())
	}

	pass, err := getStringArg(args, 2, "pgp.SignFile", false)
	if err != nil {
		return errArrResult(err.Error())
	}

	entityList, err := parseArmoredKey(privKeyArmor)
	if err != nil {
		return errArrResult("Privater Schlüssel: " + err.Error())
	}

	if err := decryptEntity(entityList[0], pass); err != nil {
		return errArrResult(err.Error())
	}

	f, err := os.Open(absPath)
	if err != nil {
		return errArrResult("Datei nicht gefunden: " + err.Error())
	}
	defer f.Close()

	buf := new(bytes.Buffer)
	w, err := armor.Encode(buf, openpgp.SignatureType, nil)
	if err != nil {
		return errArrResult("Armor-Encoder fehlgeschlagen: " + err.Error())
	}

	if err := openpgp.DetachSign(w, entityList[0], f, nil); err != nil {
		w.Close()
		return errArrResult("Signierfehler: " + err.Error())
	}
	w.Close()

	return okArrResult(buf.String())
}

// ------------------------------------------------------------
// Verify / VerifyFile
// ------------------------------------------------------------

func handleVerify(args []jsonValue) []byte {

	if len(args) < 3 {
		return errArrResult("Parameter fehlen: pgp.Verify(msg, sigArmor, pubKey)")
	}

	msg, err := getStringArg(args, 0, "pgp.Verify", true)
	if err != nil {
		return errArrResult(err.Error())
	}
	cleanMsg := strings.TrimSpace(msg)

	sigArmorRaw, err := getStringArg(args, 1, "pgp.Verify", true)
	if err != nil {
		return errArrResult(err.Error())
	}
	sigArmor := strings.TrimSpace(sigArmorRaw)

	pubKey, err := getStringArg(args, 2, "pgp.Verify", true)
	if err != nil {
		return errArrResult(err.Error())
	}

	entityList, err := parseArmoredKey(pubKey)
	if err != nil {
		return errArrResult("Public Key: " + err.Error())
	}

	block, err := armor.Decode(strings.NewReader(sigArmor))
	if err != nil {
		return errArrResult("Ungültiges Signatur-Format: " + err.Error())
	}

	_, err = openpgp.CheckDetachedSignature(entityList, strings.NewReader(cleanMsg), block.Body, nil)
	if err != nil {
		return errArrResult(classifyVerifyError(err))
	}

	return okArrResult("Gültig")
}

func handleVerifyFile(args []jsonValue) []byte {

	if len(args) < 3 {
		return errArrResult("Parameter fehlen: pgp.VerifyFile(filePath, sigArmor, pubKeyArmor)")
	}

	filePath, err := getStringArg(args, 0, "pgp.VerifyFile", true)
	if err != nil {
		return errArrResult(err.Error())
	}

	absPath, err := absPathStrict(filePath)
	if err != nil {
		return errArrResult(fmt.Sprintf("ungültiger Pfad '%s'", filePath))
	}

	sigArmor, err := getStringArg(args, 1, "pgp.VerifyFile", true)
	if err != nil {
		return errArrResult(err.Error())
	}

	pubKeyArmor, err := getStringArg(args, 2, "pgp.VerifyFile", true)
	if err != nil {
		return errArrResult(err.Error())
	}

	entityList, err := parseArmoredKey(pubKeyArmor)
	if err != nil {
		return errArrResult("Public Key: " + err.Error())
	}

	block, err := armor.Decode(strings.NewReader(sigArmor))
	if err != nil {
		return errArrResult("Ungültiges Signatur-Format: " + err.Error())
	}

	f, err := os.Open(absPath)
	if err != nil {
		return errArrResult("Datei nicht gefunden: " + err.Error())
	}
	defer f.Close()

	signer, err := openpgp.CheckDetachedSignature(entityList, f, block.Body, nil)
	if err != nil {
		return errArrResult(classifyVerifyError(err))
	}

	signerName := "Unbekannt"
	for _, id := range signer.Identities {
		signerName = id.Name
		break
	}

	return okArrResult("Gültig: " + signerName)
}

// ------------------------------------------------------------
// ChangePassword
// ------------------------------------------------------------

func handleChangePassword(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult(
			"Parameter fehlen: pgp.ChangePassword(keyFolder, [oldPass], [newPass])",
		)
	}

	folder, err := getStringArg(args, 0, "pgp.ChangePassword", true)
	if err != nil {
		return errorResult(err.Error())
	}

	oldPass, err := getStringArg(args, 1, "pgp.ChangePassword", false)
	if err != nil {
		return errorResult(err.Error())
	}

	newPass, err := getStringArg(args, 2, "pgp.ChangePassword", false)
	if err != nil {
		return errorResult(err.Error())
	}

	absFolder, err := absPathStrict(folder)
	if err != nil {
		return errorResult(fmt.Sprintf("ungültiger Pfad '%s'", folder))
	}

	keyPath := filepath.Join(absFolder, "id_pgp")
	entityList, err := parseArmoredKey(keyPath)
	if err != nil {
		return errorResult("Schlüssel nicht lesbar: " + err.Error())
	}

	entity := entityList[0]

	if err := decryptEntity(entity, oldPass); err != nil {
		return errorResult(err.Error())
	}

	if err := encryptEntity(entity, newPass); err != nil {
		return errorResult(err.Error())
	}

	if err := writeKeyFile(keyPath, openpgp.PrivateKeyType, entity); err != nil {
		return errorResult("Speichern fehlgeschlagen: " + err.Error())
	}

	return strResult("OK")
}

// ------------------------------------------------------------
// ExportKey / ImportKey / GetPublicKey
// ------------------------------------------------------------

func handleExportKey(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("Parameter fehlen: pgp.ExportKey(privKeyArmor, path, [password])")
	}

	privKeyArmor, err := getStringArg(args, 0, "pgp.ExportKey", true)
	if err != nil {
		return errorResult(err.Error())
	}

	path, err := getStringArg(args, 1, "pgp.ExportKey", true)
	if err != nil {
		return errorResult(err.Error())
	}

	pass, err := getStringArg(args, 2, "pgp.ExportKey", false)
	if err != nil {
		return errorResult(err.Error())
	}

	absP, err := absPathStrict(path)
	if err != nil {
		return errorResult(fmt.Sprintf("ungültiger Pfad '%s'", path))
	}

	entityList, err := parseArmoredKey(privKeyArmor)
	if err != nil {
		return errorResult("Ungültiger PGP-Schlüssel: " + err.Error())
	}

	entity := entityList[0]

	if err := encryptEntity(entity, pass); err != nil {
		return errorResult(err.Error())
	}

	if err := writeKeyFile(absP, openpgp.PrivateKeyType, entity); err != nil {
		return errorResult("Export fehlgeschlagen: " + err.Error())
	}

	return strResult("OK")
}

func handleImportKey(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult("Parameter fehlen: pgp.ImportKey(path, [password])")
	}

	path, err := getStringArg(args, 0, "pgp.ImportKey", true)
	if err != nil {
		return errorResult(err.Error())
	}

	pass, err := getStringArg(args, 1, "pgp.ImportKey", false)
	if err != nil {
		return errorResult(err.Error())
	}

	entityList, err := parseArmoredKey(path)
	if err != nil {
		return errorResult("Schlüssel nicht lesbar: " + err.Error())
	}

	entity := entityList[0]

	if entity.PrivateKey != nil && entity.PrivateKey.Encrypted {
		if err := decryptEntity(entity, pass); err != nil {
			return errorResult(err.Error())
		}
	}

	result, err := serializePrivateKey(entity)
	if err != nil {
		return errorResult("Serialisierung fehlgeschlagen: " + err.Error())
	}

	return strResult(result)
}

func handleGetPublicKey(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult("Parameter fehlt: pgp.GetPublicKey(pathOrKey)")
	}

	pathOrKey, err := getStringArg(args, 0, "pgp.GetPublicKey", true)
	if err != nil {
		return errorResult(err.Error())
	}

	entityList, err := parseArmoredKey(pathOrKey)
	if err != nil {
		return errorResult("Schlüssel nicht lesbar: " + err.Error())
	}

	result, err := serializePublicKey(entityList[0])
	if err != nil {
		return errorResult("Extrahierung fehlgeschlagen: " + err.Error())
	}

	return strResult(result)
}

// ------------------------------------------------------------

func main() {}
