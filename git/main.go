package main

import (
	"encoding/json"
	"unsafe"
)

// ------------------------------------------------------------
// Speicherverwaltung
// ------------------------------------------------------------

var liveBuffers = map[uint32][]byte{}

//export alloc
func alloc(size uint32) uint32 {
	buf := make([]byte, size)

	if size == 0 {
		buf = make([]byte, 1)
	}

	ptr := uint32(uintptr(unsafe.Pointer(&buf[0])))
	liveBuffers[ptr] = buf

	return ptr
}

//export dealloc
func dealloc(ptr uint32, size uint32) {
	delete(liveBuffers, ptr)
}

//export vbx_abi_version
func vbx_abi_version() int32 {
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
// Speicher-Hilfsfunktionen
// ------------------------------------------------------------

func packBytes(data []byte) uint64 {
	ptr := alloc(uint32(len(data)))

	if len(data) > 0 {
		copy(liveBuffers[ptr], data)
	}

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

// ------------------------------------------------------------
// Ergebnis-Hilfsfunktionen
// ------------------------------------------------------------

func boolResult(v bool) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "bool",
		Bool: v,
	})

	return data
}

func stringResult(v string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "str",
		Str:  v,
	})

	return data
}

func arrayResult(v []jsonValue) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "arr",
		Arr:  v,
	})

	return data
}

func mapResult(v map[string]jsonValue) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "map",
		Map:  v,
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

func valueResult(v jsonValue) []byte {
	data, _ := json.Marshal(v)
	return data
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//export vbx_describe
func vbx_describe() uint64 {

	entries := []funcDesc{

		{
			Namespace:   "git",
			Name:        "Clone",
			Params:      "url [, path]",
			Description: "Klont ein Git-Repository ohne Authentifizierung. Rückgabe: Bool - true bei Erfolg",
		},
		{
			Namespace:   "git",
			Name:        "CloneWithToken",
			Params:      "url, token [, path] [, username]",
			Description: "Klont ein Git-Repository per HTTPS mit Token-Authentifizierung. Rückgabe: Bool - true bei Erfolg",
		},
		{
			Namespace:   "git",
			Name:        "CloneWithKey",
			Params:      "url, keyPath [, path] [, knownHostsPath]",
			Description: "Klont ein Git-Repository per SSH mit Schlüssel-Authentifizierung. Rückgabe: Bool - true bei Erfolg",
		},
		{
			Namespace:   "git",
			Name:        "Pull",
			Params:      "[path] [, keyPath] [, knownHostsPath] [, token] [, username]",
			Description: "Holt Änderungen vom Remote und führt sie in den aktuellen Branch zusammen. Rückgabe: Bool - true bei Erfolg",
		},
		{
			Namespace:   "git",
			Name:        "Push",
			Params:      "[path] [, keyPath] [, knownHostsPath] [, token] [, username]",
			Description: "Sendet lokale Commits an den Remote. Rückgabe: Bool - true bei Erfolg",
		},
		{
			Namespace:   "git",
			Name:        "Fetch",
			Params:      "[path] [, keyPath] [, knownHostsPath] [, token] [, username]",
			Description: "Holt Änderungen vom Remote, ohne sie zu übernehmen. Rückgabe: Bool - true bei Erfolg",
		},
		{
			Namespace:   "git",
			Name:        "Add",
			Params:      "pattern [, path]",
			Description: "Fügt Dateien zum Git-Index hinzu (staging). Rückgabe: Bool - true bei Erfolg",
		},
		{
			Namespace:   "git",
			Name:        "Commit",
			Params:      "message [, path]",
			Description: "Erstellt einen Commit mit allen gestagten Änderungen. Rückgabe: String - Hash des erzeugten Commits",
		},
		{
			Namespace:   "git",
			Name:        "Remove",
			Params:      "pattern [, path]",
			Description: "Entfernt eine Datei aus Git-Index und Arbeitsverzeichnis. Rückgabe: Bool - true bei Erfolg",
		},
		{
			Namespace:   "git",
			Name:        "QuickPush",
			Params:      "message [, pattern] [, path] [, keyPath] [, knownHostsPath] [, token] [, username]",
			Description: "Stagt alle Änderungen, erstellt einen Commit und pusht ihn – in einem Aufruf. Rückgabe: String - Hash des erzeugten Commits",
		},
		{
			Namespace:   "git",
			Name:        "Status",
			Params:      "[path]",
			Description: "Liefert den Status des Arbeitsverzeichnisses. Rückgabe: Map mit Arrays 'modified', 'added', 'deleted', 'untracked'",
		},
		{
			Namespace:   "git",
			Name:        "Log",
			Params:      "[limit] [, path]",
			Description: "Liefert die Commit-Historie. Rückgabe: Array von Maps mit 'hash', 'author', 'message', 'date'",
		},
		{
			Namespace:   "git",
			Name:        "Diff",
			Params:      "commitA [, commitB] [, path]",
			Description: "Zeigt den Diff zwischen zwei Commits. Rückgabe: String - unified Diff",
		},
		{
			Namespace:   "git",
			Name:        "ResetHard",
			Params:      "commitHash [, path]",
			Description: "Setzt das Repository hart auf einen bestimmten Commit zurück. Rückgabe: Bool - true bei Erfolg",
		},
		{
			Namespace:   "git",
			Name:        "Reset",
			Params:      "[mode] [, path]",
			Description: "Hebt das Staging von Änderungen auf, ohne Dateien im Arbeitsverzeichnis zu verändern. Rückgabe: Bool - true bei Erfolg",
		},
		{
			Namespace:   "git",
			Name:        "CurrentBranch",
			Params:      "[path]",
			Description: "Liefert den Namen des aktuell ausgecheckten Branches.",
		},
		{
			Namespace:   "git",
			Name:        "Checkout",
			Params:      "branch [, create] [, path]",
			Description: "Wechselt den Branch oder erstellt einen neuen. Rückgabe: Bool - true bei Erfolg",
		},
		{
			Namespace:   "git",
			Name:        "IsRepo",
			Params:      "[path]",
			Description: "Prüft, ob ein Verzeichnis ein gültiges Git-Repository ist. Rückgabe: Bool - true, wenn gültiges Repository",
		},
	}

	data, _ := json.Marshal(entries)

	return packBytes(data)
}

// ------------------------------------------------------------
// vbx_call
// ------------------------------------------------------------

//export vbx_call
func vbx_call(
	namePtr,
	nameLen,
	argsPtr,
	argsLen uint32,
) uint64 {

	name := string(readBytes(namePtr, nameLen))
	argsJSON := readBytes(argsPtr, argsLen)

	var args []jsonValue

	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return packBytes(errorResult(
			"ungültige Argumente: " + err.Error(),
		))
	}

	switch name {

	case "Clone":
		return packBytes(handleClone(args))

	case "CloneWithToken":
		return packBytes(handleCloneWithToken(args))

	case "CloneWithKey":
		return packBytes(handleCloneWithKey(args))

	case "Pull":
		return packBytes(handlePull(args))

	case "Push":
		return packBytes(handlePush(args))

	case "Fetch":
		return packBytes(handleFetch(args))

	case "Add":
		return packBytes(handleAdd(args))

	case "Commit":
		return packBytes(handleCommit(args))

	case "Remove":
		return packBytes(handleRemove(args))

	case "QuickPush":
		return packBytes(handleQuickPush(args))

	case "Status":
		return packBytes(handleStatus(args))

	case "Log":
		return packBytes(handleLog(args))

	case "Diff":
		return packBytes(handleDiff(args))

	case "ResetHard":
		return packBytes(handleResetHard(args))

	case "Reset":
		return packBytes(handleReset(args))

	case "CurrentBranch":
		return packBytes(handleCurrentBranch(args))

	case "Checkout":
		return packBytes(handleCheckout(args))

	case "IsRepo":
		return packBytes(handleIsRepo(args))

	default:
		return packBytes(errorResult(
			"unbekannte Funktion: " + name,
		))
	}
}

func main() {}
