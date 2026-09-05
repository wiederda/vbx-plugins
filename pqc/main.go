package main

import (
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"unsafe"

	circlkem "github.com/cloudflare/circl/kem/mlkem/mlkem768"
	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"golang.org/x/crypto/argon2"
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

func boolResult(value bool) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "bool",
		Bool: value,
	})

	return data
}

func strArrResult(values ...string) []byte {
	arr := make([]jsonValue, len(values))

	for i, v := range values {
		arr[i] = jsonValue{Type: "str", Str: v}
	}

	data, _ := json.Marshal(jsonValue{
		Type: "arr",
		Arr:  arr,
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

func getBoolArg(
	args []jsonValue,
	idx int,
	defaultValue bool,
	funcName string,
) (bool, error) {

	if len(args) <= idx {
		return defaultValue, nil
	}

	// ToBool nativ akzeptiert vermutlich auch Strings/Zahlen als "wahrheitswertig" -
	// hier bewusst strikt auf bool beschränkt, da die einzige bekannte Nutzung
	// (SetupIdentity/SetupSignIdentity: [folder, encrypt]) ein echtes Bool erwartet.
	if args[idx].Type != "bool" {
		return false, fmt.Errorf(
			"%s: Argument %d muss ein Boolean sein",
			funcName,
			idx+1,
		)
	}

	return args[idx].Bool, nil
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

func decodeB64(s, label string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%s: ungültiges Base64", label)
	}
	return b, nil
}

// ------------------------------------------------------------
// Verschlüsselung / Datei-Logik (1:1 aus stdlib_pqc.go übernommen)
// ------------------------------------------------------------

func encryptAndSaveKey(privKeyB64, absPath, password string) (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%^&*"

	if password == "" {
		passBytes := make([]byte, 25)
		for i := range passBytes {
			n, err := crand.Int(crand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				return "", fmt.Errorf("passwort-generierung fehlgeschlagen: %w", err)
			}
			passBytes[i] = charset[n.Int64()]
		}
		password = string(passBytes)
	}

	salt := make([]byte, 16)
	if _, err := io.ReadFull(crand.Reader, salt); err != nil {
		return "", fmt.Errorf("salt-generierung fehlgeschlagen: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher-initialisierung fehlgeschlagen: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("GCM-initialisierung fehlgeschlagen: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce-generierung fehlgeschlagen: %w", err)
	}

	aad := []byte("PQC-KEYSTORE-v1")
	ciphertext := gcm.Seal(nil, nonce, []byte(privKeyB64), aad)

	out := make([]byte, 0, 4+len(salt)+len(nonce)+len(ciphertext))
	out = append(out, []byte("PQC1")...)
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)

	tmpFile, err := os.CreateTemp(
		filepath.Dir(absPath),
		filepath.Base(absPath)+".tmp-*",
	)
	if err != nil {
		return "", fmt.Errorf("temp-datei konnte nicht erstellt werden: %w", err)
	}

	if _, err := tmpFile.Write(out); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("schreiben fehlgeschlagen: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("sync fehlgeschlagen: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("close fehlgeschlagen: %w", err)
	}

	if err := os.Rename(tmpFile.Name(), absPath); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("umbenennen fehlgeschlagen: %w", err)
	}

	return password, nil
}

func decryptFile(absPath, password string) (string, error) {
	blob, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("datei nicht lesbar: %w", err)
	}

	if len(blob) < 4 || string(blob[:4]) != "PQC1" {
		return "", fmt.Errorf("unbekanntes dateiformat (kein PQC1-Header)")
	}
	blob = blob[4:]

	if len(blob) < 16 {
		return "", fmt.Errorf("datei korrupt: zu kurz für salt")
	}
	salt := blob[:16]
	blob = blob[16:]

	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher-initialisierung fehlgeschlagen: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("GCM-initialisierung fehlgeschlagen: %w", err)
	}

	ns := gcm.NonceSize()
	if len(blob) < ns {
		return "", fmt.Errorf("datei korrupt: zu kurz für nonce")
	}

	plaintext, err := gcm.Open(nil, blob[:ns], blob[ns:], []byte("PQC-KEYSTORE-v1"))
	if err != nil {
		return "", fmt.Errorf("passwort falsch oder datei manipuliert")
	}

	return string(plaintext), nil
}

func hashFileSHA256(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("datei nicht lesbar: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, fmt.Errorf("hash-berechnung fehlgeschlagen: %w", err)
	}

	return h.Sum(nil), nil
}

func printPasswordWarning(keyPath string) {
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║         WICHTIG: MASTER-PASSWORT                ║")
	fmt.Println("╠══════════════════════════════════════════════════╣")
	fmt.Println("║  Das Passwort wurde gespeichert unter:          ║")
	fmt.Printf("║  %-49s║\n", keyPath)
	fmt.Println("╠══════════════════════════════════════════════════╣")
	fmt.Println("║  1. Öffne die Datei und notiere das Passwort.   ║")
	fmt.Println("║  2. Bewahre es sicher auf (z.B. Passwortmanager)║")
	fmt.Println("║  3. Lösche die Datei anschließend.              ║")
	fmt.Println("║                                                  ║")
	fmt.Println("║  Ohne dieses Passwort ist der Key VERLOREN.     ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
}

func printUnencryptedWarning(filename string) {
	fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	fmt.Println("⚠️  SICHERHEITSWARNUNG: PRIVATE KEY UNVERSCHLÜSSELT!  ⚠️")
	fmt.Printf(" Die Datei %s ist NICHT geschützt. Bitte verschlüsseln!\n", filename)
	fmt.Println(" Jeder der Zugriff auf diese Datei hat, kann alles lesen!")
	fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
}

func setupIdentityFiles(absFolder, privB64, pubB64, keyName string, shouldEncrypt bool) error {
	if err := os.MkdirAll(absFolder, 0700); err != nil {
		return fmt.Errorf("ordner konnte nicht erstellt werden: %w", err)
	}

	pubPath := filepath.Join(absFolder, keyName+".pub")
	if err := os.WriteFile(pubPath, []byte(pubB64), 0644); err != nil {
		return fmt.Errorf("public key konnte nicht gespeichert werden: %w", err)
	}

	privPath := filepath.Join(absFolder, keyName)

	if shouldEncrypt {
		keyFilePath := filepath.Join(absFolder, keyName+".key")

		pass, err := encryptAndSaveKey(privB64, privPath, "")
		if err != nil {
			os.Remove(pubPath)
			return err
		}

		if err := os.WriteFile(keyFilePath, []byte(pass), 0600); err != nil {
			os.Remove(pubPath)
			os.Remove(privPath)
			return fmt.Errorf("passwort-datei konnte nicht geschrieben werden: %w", err)
		}

		printPasswordWarning(keyFilePath)
	} else {
		if err := os.WriteFile(privPath, []byte(privB64), 0600); err != nil {
			os.Remove(pubPath)
			return fmt.Errorf("private key konnte nicht gespeichert werden: %w", err)
		}

		printUnencryptedWarning(keyName)
	}

	return nil
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {

	entries := []funcDesc{
		{
			Namespace:   "pqc",
			Name:        "GenerateKeyPair",
			Params:      "-",
			Description: "Erzeugt ein ML-KEM-768 Schlüsselpaar. Rückgabe: [pubB64, privB64].",
		},
		{
			Namespace:   "pqc",
			Name:        "GenerateSigKeyPair",
			Params:      "-",
			Description: "Erzeugt ein ML-DSA-65 Signatur-Schlüsselpaar. Rückgabe: [pubB64, privB64].",
		},
		{
			Namespace:   "pqc",
			Name:        "Encapsulate",
			Params:      "pubKeyB64",
			Description: "Erzeugt ein Shared Secret für einen ML-KEM Public Key. Rückgabe: [ciphertextB64, sharedSecretB64].",
		},
		{
			Namespace:   "pqc",
			Name:        "Decapsulate",
			Params:      "ciphertextB64, privKeyB64",
			Description: "Entkapselt ein Shared Secret mit einem ML-KEM Private Key.",
		},
		{
			Namespace:   "pqc",
			Name:        "Sign",
			Params:      "msg, privKeyB64",
			Description: "Signiert eine Nachricht mit ML-DSA-65.",
		},
		{
			Namespace:   "pqc",
			Name:        "Verify",
			Params:      "msg, sigB64, pubKeyB64",
			Description: "Prüft eine ML-DSA-65 Signatur gegen eine Nachricht.",
		},
		{
			Namespace:   "pqc",
			Name:        "SignFile",
			Params:      "filePath, privKeyB64",
			Description: "Signiert eine Datei (über ihren SHA-256-Hash) mit ML-DSA-65.",
		},
		{
			Namespace:   "pqc",
			Name:        "VerifyFile",
			Params:      "filePath, sigB64, pubKeyB64",
			Description: "Verifiziert eine Datei gegen eine ML-DSA-65 Signatur.",
		},
		{
			Namespace:   "pqc",
			Name:        "SetupIdentity",
			Params:      "folder, [encrypt]",
			Description: "Erzeugt ein ML-KEM-768 Schlüsselpaar und speichert es im Ordner.",
		},
		{
			Namespace:   "pqc",
			Name:        "SetupSignIdentity",
			Params:      "folder, [encrypt]",
			Description: "Erzeugt ein ML-DSA-65 Signatur-Schlüsselpaar und speichert es im Ordner.",
		},
		{
			Namespace:   "pqc",
			Name:        "ExportKey",
			Params:      "privKeyB64, path, [password]",
			Description: "Exportiert einen ML-KEM Private Key verschlüsselt in eine Datei.",
		},
		{
			Namespace:   "pqc",
			Name:        "ImportKey",
			Params:      "path, [password]",
			Description: "Lädt einen ML-KEM Private Key aus einer Datei (optional entschlüsselt).",
		},
		{
			Namespace:   "pqc",
			Name:        "ExportSignKey",
			Params:      "privKeyB64, path, [password]",
			Description: "Exportiert einen ML-DSA Private Key verschlüsselt in eine Datei.",
		},
		{
			Namespace:   "pqc",
			Name:        "ImportSignKey",
			Params:      "path, [password]",
			Description: "Lädt einen ML-DSA Private Key aus einer Datei (optional entschlüsselt).",
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

	case "GenerateKeyPair":
		return packBytes(handleGenerateKeyPair(args))

	case "GenerateSigKeyPair":
		return packBytes(handleGenerateSigKeyPair(args))

	case "Encapsulate":
		return packBytes(handleEncapsulate(args))

	case "Decapsulate":
		return packBytes(handleDecapsulate(args))

	case "Sign":
		return packBytes(handleSign(args))

	case "Verify":
		return packBytes(handleVerify(args))

	case "SignFile":
		return packBytes(handleSignFile(args))

	case "VerifyFile":
		return packBytes(handleVerifyFile(args))

	case "SetupIdentity":
		return packBytes(handleSetupIdentity(args))

	case "SetupSignIdentity":
		return packBytes(handleSetupSignIdentity(args))

	case "ExportKey":
		return packBytes(handleExportKey(args))

	case "ImportKey":
		return packBytes(handleImportKey(args))

	case "ExportSignKey":
		return packBytes(handleExportSignKey(args))

	case "ImportSignKey":
		return packBytes(handleImportSignKey(args))

	default:
		return packBytes(
			errorResult(
				"unbekannte Funktion: " + name,
			),
		)
	}
}

// ------------------------------------------------------------
// GenerateKeyPair / GenerateSigKeyPair
// ------------------------------------------------------------

func handleGenerateKeyPair(args []jsonValue) []byte {

	pub, priv, err := circlkem.GenerateKeyPair(nil)
	if err != nil {
		return errorResult("Schlüsselgenerierung fehlgeschlagen: " + err.Error())
	}

	pubBytes, err := pub.MarshalBinary()
	if err != nil {
		return errorResult("Public Key serialisierung fehlgeschlagen: " + err.Error())
	}

	privBytes, err := priv.MarshalBinary()
	if err != nil {
		return errorResult("Private Key serialisierung fehlgeschlagen: " + err.Error())
	}

	return strArrResult(
		base64.StdEncoding.EncodeToString(pubBytes),
		base64.StdEncoding.EncodeToString(privBytes),
	)
}

func handleGenerateSigKeyPair(args []jsonValue) []byte {

	scheme := mldsa65.Scheme()
	pk, sk, err := scheme.GenerateKey()
	if err != nil {
		return errorResult("Schlüsselgenerierung fehlgeschlagen: " + err.Error())
	}

	pubBytes, err := pk.MarshalBinary()
	if err != nil {
		return errorResult("Public Key serialisierung fehlgeschlagen: " + err.Error())
	}

	privBytes, err := sk.MarshalBinary()
	if err != nil {
		return errorResult("Private Key serialisierung fehlgeschlagen: " + err.Error())
	}

	return strArrResult(
		base64.StdEncoding.EncodeToString(pubBytes),
		base64.StdEncoding.EncodeToString(privBytes),
	)
}

// ------------------------------------------------------------
// Encapsulate / Decapsulate
// ------------------------------------------------------------

func handleEncapsulate(args []jsonValue) []byte {

	pubKeyB64, err := getStringArg(args, 0, "pqc.Encapsulate", true)
	if err != nil {
		return errorResult(err.Error())
	}

	pubBytes, err := decodeB64(pubKeyB64, "Public Key")
	if err != nil {
		return errorResult(err.Error())
	}

	scheme := circlkem.Scheme()
	pubKey, err := scheme.UnmarshalBinaryPublicKey(pubBytes)
	if err != nil {
		return errorResult("Public Key ungültig: " + err.Error())
	}

	ciphertext, sharedSecret, err := scheme.Encapsulate(pubKey)
	if err != nil {
		return errorResult("Encapsulate fehlgeschlagen: " + err.Error())
	}

	return strArrResult(
		base64.StdEncoding.EncodeToString(ciphertext),
		base64.StdEncoding.EncodeToString(sharedSecret),
	)
}

func handleDecapsulate(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("Parameter fehlen: pqc.Decapsulate(ciphertextB64, privKeyB64)")
	}

	ciphertextB64, err := getStringArg(args, 0, "pqc.Decapsulate", true)
	if err != nil {
		return errorResult(err.Error())
	}

	privKeyB64, err := getStringArg(args, 1, "pqc.Decapsulate", true)
	if err != nil {
		return errorResult(err.Error())
	}

	ctBytes, err := decodeB64(ciphertextB64, "Ciphertext")
	if err != nil {
		return errorResult(err.Error())
	}

	skBytes, err := decodeB64(privKeyB64, "Private Key")
	if err != nil {
		return errorResult(err.Error())
	}

	scheme := circlkem.Scheme()
	sk, err := scheme.UnmarshalBinaryPrivateKey(skBytes)
	if err != nil {
		return errorResult("Private Key ungültig: " + err.Error())
	}

	ss, err := scheme.Decapsulate(sk, ctBytes)
	if err != nil {
		return errorResult("Decapsulate fehlgeschlagen: " + err.Error())
	}

	return strResult(base64.StdEncoding.EncodeToString(ss))
}

// ------------------------------------------------------------
// Sign / Verify
// ------------------------------------------------------------

func handleSign(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("Parameter fehlen: pqc.Sign(msg, privKeyB64)")
	}

	msg, err := getStringArg(args, 0, "pqc.Sign", true)
	if err != nil {
		return errorResult(err.Error())
	}

	privKeyB64, err := getStringArg(args, 1, "pqc.Sign", true)
	if err != nil {
		return errorResult(err.Error())
	}

	skBytes, err := decodeB64(privKeyB64, "Private Key")
	if err != nil {
		return errorResult(err.Error())
	}

	scheme := mldsa65.Scheme()
	sk, err := scheme.UnmarshalBinaryPrivateKey(skBytes)
	if err != nil {
		return errorResult("Private Key ungültig: " + err.Error())
	}

	sig := scheme.Sign(sk, []byte(msg), nil)

	return strResult(base64.StdEncoding.EncodeToString(sig))
}

func handleVerify(args []jsonValue) []byte {

	if len(args) < 3 {
		return errorResult("Parameter fehlen: pqc.Verify(msg, sigB64, pubKeyB64)")
	}

	msg, err := getStringArg(args, 0, "pqc.Verify", true)
	if err != nil {
		return errorResult(err.Error())
	}

	sigB64, err := getStringArg(args, 1, "pqc.Verify", true)
	if err != nil {
		return errorResult(err.Error())
	}

	pubKeyB64, err := getStringArg(args, 2, "pqc.Verify", true)
	if err != nil {
		return errorResult(err.Error())
	}

	sig, err := decodeB64(sigB64, "Signatur")
	if err != nil {
		return errorResult(err.Error())
	}

	pubBytes, err := decodeB64(pubKeyB64, "Public Key")
	if err != nil {
		return errorResult(err.Error())
	}

	scheme := mldsa65.Scheme()
	pk, err := scheme.UnmarshalBinaryPublicKey(pubBytes)
	if err != nil {
		return errorResult("Public Key ungültig: " + err.Error())
	}

	if !scheme.Verify(pk, []byte(msg), sig, nil) {
		return errorResult("Signatur ungültig")
	}

	return boolResult(true)
}

// ------------------------------------------------------------
// SignFile / VerifyFile
// ------------------------------------------------------------

func handleSignFile(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("Parameter fehlen: pqc.SignFile(filePath, privKeyB64)")
	}

	filePath, err := getStringArg(args, 0, "pqc.SignFile", true)
	if err != nil {
		return errorResult(err.Error())
	}

	absPath, err := absPathStrict(filePath)
	if err != nil {
		return errorResult(fmt.Sprintf("ungültiger Pfad '%s'", filePath))
	}

	privKeyB64, err := getStringArg(args, 1, "pqc.SignFile", true)
	if err != nil {
		return errorResult(err.Error())
	}

	fileHash, err := hashFileSHA256(absPath)
	if err != nil {
		return errorResult(err.Error())
	}

	skBytes, err := decodeB64(privKeyB64, "Private Key")
	if err != nil {
		return errorResult(err.Error())
	}

	scheme := mldsa65.Scheme()
	sk, err := scheme.UnmarshalBinaryPrivateKey(skBytes)
	if err != nil {
		return errorResult("Private Key ungültig: " + err.Error())
	}

	sig := scheme.Sign(sk, fileHash, nil)

	return strResult(base64.StdEncoding.EncodeToString(sig))
}

func handleVerifyFile(args []jsonValue) []byte {

	if len(args) < 3 {
		return errorResult("Parameter fehlen: pqc.VerifyFile(filePath, sigB64, pubKeyB64)")
	}

	filePath, err := getStringArg(args, 0, "pqc.VerifyFile", true)
	if err != nil {
		return errorResult(err.Error())
	}

	absPath, err := absPathStrict(filePath)
	if err != nil {
		return errorResult(fmt.Sprintf("ungültiger Pfad '%s'", filePath))
	}

	sigB64, err := getStringArg(args, 1, "pqc.VerifyFile", true)
	if err != nil {
		return errorResult(err.Error())
	}

	pubKeyB64, err := getStringArg(args, 2, "pqc.VerifyFile", true)
	if err != nil {
		return errorResult(err.Error())
	}

	fileHash, err := hashFileSHA256(absPath)
	if err != nil {
		return errorResult(err.Error())
	}

	sigBytes, err := decodeB64(sigB64, "Signatur")
	if err != nil {
		return errorResult(err.Error())
	}

	pkBytes, err := decodeB64(pubKeyB64, "Public Key")
	if err != nil {
		return errorResult(err.Error())
	}

	scheme := mldsa65.Scheme()
	pk, err := scheme.UnmarshalBinaryPublicKey(pkBytes)
	if err != nil {
		return errorResult("Public Key ungültig: " + err.Error())
	}

	if !scheme.Verify(pk, fileHash, sigBytes, nil) {
		return errorResult("Signatur stimmt nicht mit Datei überein")
	}

	return boolResult(true)
}

// ------------------------------------------------------------
// SetupIdentity / SetupSignIdentity
// ------------------------------------------------------------

func handleSetupIdentity(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult("Parameter fehlen: pqc.SetupIdentity(folder, [encrypt])")
	}

	folder, err := getStringArg(args, 0, "pqc.SetupIdentity", true)
	if err != nil {
		return errorResult(err.Error())
	}

	absFolder, err := absPathStrict(folder)
	if err != nil {
		return errorResult(fmt.Sprintf("ungültiger Pfad '%s'", folder))
	}

	shouldEncrypt, err := getBoolArg(args, 1, true, "pqc.SetupIdentity")
	if err != nil {
		return errorResult(err.Error())
	}

	pub, priv, err := circlkem.GenerateKeyPair(nil)
	if err != nil {
		return errorResult("Schlüsselgenerierung fehlgeschlagen: " + err.Error())
	}

	pubBytes, err := pub.MarshalBinary()
	if err != nil {
		return errorResult("Public Key serialisierung fehlgeschlagen: " + err.Error())
	}

	privBytes, err := priv.MarshalBinary()
	if err != nil {
		return errorResult("Private Key serialisierung fehlgeschlagen: " + err.Error())
	}

	pubB64 := base64.StdEncoding.EncodeToString(pubBytes)
	privB64 := base64.StdEncoding.EncodeToString(privBytes)

	if err := setupIdentityFiles(absFolder, privB64, pubB64, "id_pqc", shouldEncrypt); err != nil {
		return errorResult(err.Error())
	}

	return strResult("OK")
}

func handleSetupSignIdentity(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult("Parameter fehlen: pqc.SetupSignIdentity(folder, [encrypt])")
	}

	folder, err := getStringArg(args, 0, "pqc.SetupSignIdentity", true)
	if err != nil {
		return errorResult(err.Error())
	}

	absFolder, err := absPathStrict(folder)
	if err != nil {
		return errorResult(fmt.Sprintf("ungültiger Pfad '%s'", folder))
	}

	shouldEncrypt, err := getBoolArg(args, 1, true, "pqc.SetupSignIdentity")
	if err != nil {
		return errorResult(err.Error())
	}

	scheme := mldsa65.Scheme()
	pk, sk, err := scheme.GenerateKey()
	if err != nil {
		return errorResult("Schlüsselgenerierung fehlgeschlagen: " + err.Error())
	}

	pubBytes, err := pk.MarshalBinary()
	if err != nil {
		return errorResult("Public Key serialisierung fehlgeschlagen: " + err.Error())
	}

	privBytes, err := sk.MarshalBinary()
	if err != nil {
		return errorResult("Private Key serialisierung fehlgeschlagen: " + err.Error())
	}

	pubB64 := base64.StdEncoding.EncodeToString(pubBytes)
	privB64 := base64.StdEncoding.EncodeToString(privBytes)

	if err := setupIdentityFiles(absFolder, privB64, pubB64, "id_sig", shouldEncrypt); err != nil {
		return errorResult(err.Error())
	}

	return strResult("OK")
}

// ------------------------------------------------------------
// ExportKey / ImportKey / ExportSignKey / ImportSignKey
// ------------------------------------------------------------

func handleExportKey(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("Parameter fehlen: pqc.ExportKey(privKeyB64, path, [password])")
	}

	privKeyB64, err := getStringArg(args, 0, "pqc.ExportKey", true)
	if err != nil {
		return errorResult(err.Error())
	}

	path, err := getStringArg(args, 1, "pqc.ExportKey", true)
	if err != nil {
		return errorResult(err.Error())
	}

	absP, err := absPathStrict(path)
	if err != nil {
		return errorResult(fmt.Sprintf("ungültiger Pfad '%s'", path))
	}

	password, err := getStringArg(args, 2, "pqc.ExportKey", false)
	if err != nil {
		return errorResult(err.Error())
	}

	if password == "" {
		if err := os.WriteFile(absP, []byte(privKeyB64), 0600); err != nil {
			return errorResult("Schreibfehler: " + err.Error())
		}
		return strResult("")
	}

	usedPassword, err := encryptAndSaveKey(privKeyB64, absP, password)
	if err != nil {
		return errorResult(err.Error())
	}

	return strResult(usedPassword)
}

func handleImportKey(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult("Parameter fehlt: pqc.ImportKey(path, [password])")
	}

	path, err := getStringArg(args, 0, "pqc.ImportKey", true)
	if err != nil {
		return errorResult(err.Error())
	}

	absP, err := absPathStrict(path)
	if err != nil {
		return errorResult(fmt.Sprintf("ungültiger Pfad '%s'", path))
	}

	password, err := getStringArg(args, 1, "pqc.ImportKey", false)
	if err != nil {
		return errorResult(err.Error())
	}

	if password != "" {
		decrypted, err := decryptFile(absP, password)
		if err != nil {
			return errorResult(err.Error())
		}
		return strResult(decrypted)
	}

	b, err := os.ReadFile(absP)
	if err != nil {
		return errorResult("Datei nicht lesbar: " + err.Error())
	}

	return strResult(string(b))
}

func handleExportSignKey(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("Parameter fehlen: pqc.ExportSignKey(privKeyB64, path, [password])")
	}

	privKeyB64, err := getStringArg(args, 0, "pqc.ExportSignKey", true)
	if err != nil {
		return errorResult(err.Error())
	}

	path, err := getStringArg(args, 1, "pqc.ExportSignKey", true)
	if err != nil {
		return errorResult(err.Error())
	}

	absP, err := absPathStrict(path)
	if err != nil {
		return errorResult(fmt.Sprintf("ungültiger Pfad '%s'", path))
	}

	password, err := getStringArg(args, 2, "pqc.ExportSignKey", false)
	if err != nil {
		return errorResult(err.Error())
	}

	usedPassword, err := encryptAndSaveKey(privKeyB64, absP, password)
	if err != nil {
		return errorResult(err.Error())
	}

	return strResult(usedPassword)
}

func handleImportSignKey(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult("Parameter fehlt: pqc.ImportSignKey(path, [password])")
	}

	path, err := getStringArg(args, 0, "pqc.ImportSignKey", true)
	if err != nil {
		return errorResult(err.Error())
	}

	absP, err := absPathStrict(path)
	if err != nil {
		return errorResult(fmt.Sprintf("ungültiger Pfad '%s'", path))
	}

	password, err := getStringArg(args, 1, "pqc.ImportSignKey", false)
	if err != nil {
		return errorResult(err.Error())
	}

	if password != "" {
		decrypted, err := decryptFile(absP, password)
		if err != nil {
			return errorResult(err.Error())
		}
		return strResult(decrypted)
	}

	b, err := os.ReadFile(absP)
	if err != nil {
		return errorResult("Datei nicht lesbar: " + err.Error())
	}

	return strResult(string(b))
}

// ------------------------------------------------------------

func main() {}
