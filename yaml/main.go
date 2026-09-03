package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unsafe"

	"gopkg.in/yaml.v3"
)

// ------------------------------------------------------------
// Speicherverwaltung (Sicher für TinyGo)
// ------------------------------------------------------------
//
// WICHTIG: liveBuffers wird bewusst NICHT mit einem Composite-Literal
// (map[uint32][]byte{}) initialisiert. Ein solches Literal erzeugt
// Go-Init-Code, der normalerweise vor main() läuft - bei diesem
// Host läuft aber kein echter WASI-Reactor-Start (_start wird
// übersprungen, siehe Kommentar unten), daher würde dieser Init-Code
// nie ausgeführt und liveBuffers bliebe eine ECHTE nil-Map.
// TinyGo's hashmapGet/hashmapBinaryGet paniken (anders als der
// normale Go-Compiler) auch beim reinen LESEN einer solchen nil-Map.
// Fix: Zero-Value-Deklaration + Lazy-Init beim ersten alloc()-Aufruf.
//
// Hintergrund _start: TinyGo ruft bei WASI-Modulen automatisch
// "_start" auf, das nach main() sofort proc_exit(0) auslöst - der
// Host instanziiert daher bewusst mit WithStartFunctions() (leer),
// um genau das zu vermeiden. Nebenwirkung: package-level Initializer,
// die echten Laufzeitcode brauchen (Maps, Slices mit Inhalt, Funktions-
// aufrufe als Initializer), laufen dadurch nie. Diese Konvention
// (Zero-Value + Lazy-Init) gilt für alle künftigen Plugins.
// ------------------------------------------------------------

var liveBuffers map[uint32][]byte

//export alloc
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

//export dealloc
func dealloc(ptr uint32, size uint32) {
	if liveBuffers == nil {
		return
	}

	delete(liveBuffers, ptr)
}

//export vbx_abi_version
func vbx_abi_version() int32 {
	return 1
}

// ------------------------------------------------------------
// Wire-Format
// Muss mit der Host-Seite übereinstimmen
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
// Speicher / Wire-Format Hilfsfunktionen
// ------------------------------------------------------------

func packBytes(data []byte) uint64 {
	ptr := alloc(uint32(len(data)))

	if len(data) > 0 {
		copy(liveBuffers[ptr], data)
	}

	return (uint64(ptr) << 32) | uint64(len(data))
}

func readBytes(ptr, length uint32) []byte {
	if liveBuffers == nil {
		return nil
	}

	buf, ok := liveBuffers[ptr]
	if !ok {
		return nil
	}

	if uint32(len(buf)) < length {
		return buf
	}

	return buf[:length]
}

func numResult(n float64) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "num",
		Num:  n,
	})

	return data
}

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
// Konvertierung jsonValue -> interface{}
// ------------------------------------------------------------

func jsonValueToInterface(v jsonValue) interface{} {
	switch v.Type {

	case "num":
		return v.Num

	case "str":
		return v.Str

	case "bool":
		return v.Bool

	case "arr":
		arr := make([]interface{}, 0, len(v.Arr))

		for _, item := range v.Arr {
			arr = append(arr, jsonValueToInterface(item))
		}

		return arr

	case "map":
		m := make(map[string]interface{})

		for key, item := range v.Map {
			m[key] = jsonValueToInterface(item)
		}

		return m

	case "bytes":
		return v.Bytes

	default:
		return nil
	}
}

// ------------------------------------------------------------
// interface{} -> jsonValue
// ------------------------------------------------------------

func interfaceToJSONValue(v interface{}) jsonValue {
	switch value := v.(type) {

	case nil:
		return jsonValue{
			Type: "null",
		}

	case float64:
		return jsonValue{
			Type: "num",
			Num:  value,
		}

	case float32:
		return jsonValue{
			Type: "num",
			Num:  float64(value),
		}

	case int:
		return jsonValue{
			Type: "num",
			Num:  float64(value),
		}

	case int8:
		return jsonValue{
			Type: "num",
			Num:  float64(value),
		}

	case int16:
		return jsonValue{
			Type: "num",
			Num:  float64(value),
		}

	case int32:
		return jsonValue{
			Type: "num",
			Num:  float64(value),
		}

	case int64:
		return jsonValue{
			Type: "num",
			Num:  float64(value),
		}

	case uint:
		return jsonValue{
			Type: "num",
			Num:  float64(value),
		}

	case uint8:
		return jsonValue{
			Type: "num",
			Num:  float64(value),
		}

	case uint16:
		return jsonValue{
			Type: "num",
			Num:  float64(value),
		}

	case uint32:
		return jsonValue{
			Type: "num",
			Num:  float64(value),
		}

	case uint64:
		return jsonValue{
			Type: "num",
			Num:  float64(value),
		}

	case string:
		return jsonValue{
			Type: "str",
			Str:  value,
		}

	case bool:
		return jsonValue{
			Type: "bool",
			Bool: value,
		}

	case []byte:
		return jsonValue{
			Type:  "bytes",
			Bytes: value,
		}

	case []interface{}:
		arr := make([]jsonValue, 0, len(value))

		for _, item := range value {
			arr = append(arr, interfaceToJSONValue(item))
		}

		return jsonValue{
			Type: "arr",
			Arr:  arr,
		}

	case map[string]interface{}:
		m := make(map[string]jsonValue)

		for key, item := range value {
			m[key] = interfaceToJSONValue(item)
		}

		return jsonValue{
			Type: "map",
			Map:  m,
		}

	default:
		return jsonValue{
			Type: "null",
		}
	}
}

// ------------------------------------------------------------
// YAML -> Wire-Format
//
// yaml.v3 kann map[string]interface{} und
// map[interface{}]interface{} liefern.
// Wir normalisieren deshalb rekursiv.
// ------------------------------------------------------------

func normalizeYAMLValue(v interface{}) interface{} {
	switch value := v.(type) {

	case map[string]interface{}:
		out := make(map[string]interface{})

		for key, item := range value {
			out[key] = normalizeYAMLValue(item)
		}

		return out

	case map[interface{}]interface{}:
		out := make(map[string]interface{})

		for key, item := range value {
			out[fmt.Sprint(key)] = normalizeYAMLValue(item)
		}

		return out

	case []interface{}:
		out := make([]interface{}, len(value))

		for i, item := range value {
			out[i] = normalizeYAMLValue(item)
		}

		return out

	default:
		return value
	}
}

// ------------------------------------------------------------
// YAML -> jsonValue
// ------------------------------------------------------------

func yamlToJSONValue(v interface{}) jsonValue {
	return interfaceToJSONValue(normalizeYAMLValue(v))
}

// ------------------------------------------------------------
// jsonValue -> YAML-kompatibles interface{}
// ------------------------------------------------------------

func jsonValueToYAML(v jsonValue) interface{} {
	switch v.Type {

	case "null":
		return nil

	case "num":
		return v.Num

	case "str":
		return v.Str

	case "bool":
		return v.Bool

	case "arr":
		arr := make([]interface{}, 0, len(v.Arr))

		for _, item := range v.Arr {
			arr = append(arr, jsonValueToYAML(item))
		}

		return arr

	case "map":
		m := make(map[string]interface{})

		for key, item := range v.Map {
			m[key] = jsonValueToYAML(item)
		}

		return m

	case "bytes":
		return v.Bytes

	default:
		return nil
	}
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//export vbx_describe
func vbx_describe() uint64 {
	desc := `[
		{
			"namespace": "yaml",
			"name": "Parse",
			"params": "yamlContent",
			"description": "Prüft die YAML-Struktur. Gibt True zurück, wenn valide, oder eine Fehlermeldung mit Zeilenangabe bei Syntaxfehlern."
		},
		{
			"namespace": "yaml",
			"name": "ParseAll",
			"params": "yamlContent",
			"description": "Prüft den gesamten YAML-Stream mit allen Dokumenten. Gibt True zurück, wenn der gesamte Inhalt valide ist."
		},
		{
			"namespace": "yaml",
			"name": "Get",
			"params": "yaml, path, v",
			"description": "Liest einen Wert aus YAML über einen Pfad."
		},
		{
			"namespace": "yaml",
			"name": "Set",
			"params": "yaml, path, val",
			"description": "Setzt einen Wert in YAML und gibt den aktualisierten YAML-String zurück."
		},
		{
			"namespace": "yaml",
			"name": "Stringify",
			"params": "value",
			"description": "Konvertiert eine VBX-Struktur aus Map oder Array in einen formatierten YAML-String."
		}
	]`

	return packBytes([]byte(desc))
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
		return packBytes(
			errorResult(
				"ungültige Argumente: " + err.Error(),
			),
		)
	}

	switch name {

	case "Parse":
		return packBytes(handleParse(args))

	case "ParseAll":
		return packBytes(handleParseAll(args))

	case "Get":
		return packBytes(handleGet(args))

	case "Set":
		return packBytes(handleSet(args))

	case "Stringify":
		return packBytes(handleStringify(args))

	default:
		return packBytes(
			errorResult(
				"unbekannte Funktion: " + name,
			),
		)
	}
}

// ------------------------------------------------------------
// yaml.Parse
// ------------------------------------------------------------

func handleParse(args []jsonValue) []byte {

	if len(args) < 1 {
		return strResult(
			"ERROR: Kein Inhalt zum Prüfen übergeben",
		)
	}

	content := ""

	if args[0].Type == "str" {
		content = args[0].Str
	} else {
		content = fmt.Sprint(
			jsonValueToInterface(args[0]),
		)
	}

	if content == "" {
		return strResult(
			"ERROR: Kein Inhalt zum Prüfen übergeben",
		)
	}

	var data interface{}

	err := yaml.Unmarshal(
		[]byte(content),
		&data,
	)

	if err != nil {
		return strResult(
			"ERROR: " + err.Error(),
		)
	}

	return boolResult(true)
}

// ------------------------------------------------------------
// yaml.ParseAll
// ------------------------------------------------------------

func handleParseAll(args []jsonValue) []byte {

	if len(args) < 1 {
		return strResult(
			"ERROR: Kein Inhalt zum Parsen übergeben",
		)
	}

	content := ""

	if args[0].Type == "str" {
		content = args[0].Str
	} else {
		content = fmt.Sprint(
			jsonValueToInterface(args[0]),
		)
	}

	if content == "" {
		return strResult(
			"ERROR: Kein Inhalt zum Parsen übergeben",
		)
	}

	decoder := yaml.NewDecoder(
		strings.NewReader(content),
	)

	for {

		var data interface{}

		err := decoder.Decode(&data)

		if err == io.EOF {
			break
		}

		if err != nil {
			return strResult(
				"ERROR: " + err.Error(),
			)
		}
	}

	return boolResult(true)
}

// ------------------------------------------------------------
// yaml.Get
// ------------------------------------------------------------

func handleGet(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult(
			"Argumente fehlen",
		)
	}

	if args[0].Type != "str" {
		return errorResult(
			"yaml.Get: YAML muss ein String sein",
		)
	}

	if args[1].Type != "str" {
		return errorResult(
			"yaml.Get: path muss ein String sein",
		)
	}

	var data interface{}

	if err := yaml.Unmarshal(
		[]byte(args[0].Str),
		&data,
	); err != nil {
		return errorResult(err.Error())
	}

	data = normalizeYAMLValue(data)

	val, ok := getValueByPath(
		data,
		splitPath(args[1].Str),
	)

	if !ok {
		// Entspricht Value{} der Host-Implementierung.
		return valueResult(jsonValue{})
	}

	return valueResult(
		interfaceToJSONValue(val),
	)
}

// ------------------------------------------------------------
// yaml.Set
// ------------------------------------------------------------

func handleSet(args []jsonValue) []byte {

	if len(args) < 3 {
		return errorResult(
			"Argumente fehlen",
		)
	}

	if args[0].Type != "str" {
		return errorResult(
			"yaml.Set: YAML muss ein String sein",
		)
	}

	if args[1].Type != "str" {
		return errorResult(
			"yaml.Set: path muss ein String sein",
		)
	}

	var data interface{}

	if err := yaml.Unmarshal(
		[]byte(args[0].Str),
		&data,
	); err != nil {
		return errorResult(err.Error())
	}

	data = normalizeYAMLValue(data)

	updated, err := setValueByPath(
		data,
		splitPath(args[1].Str),
		jsonValueToYAML(args[2]),
	)

	if err != nil {
		return errorResult(err.Error())
	}

	out, err := yaml.Marshal(updated)

	if err != nil {
		return errorResult(err.Error())
	}

	return strResult(string(out))
}

// ------------------------------------------------------------
// yaml.Stringify
// ------------------------------------------------------------

func handleStringify(args []jsonValue) []byte {

	if len(args) < 1 {
		return strResult("")
	}

	raw := jsonValueToYAML(args[0])

	out, err := yaml.Marshal(raw)

	if err != nil {
		return errorResult(err.Error())
	}

	return strResult(string(out))
}

// ------------------------------------------------------------
// GET
// Navigiert rekursiv.
// ------------------------------------------------------------

func getValueByPath(
	current interface{},
	keys []string,
) (interface{}, bool) {

	if len(keys) == 0 {
		return current, true
	}

	switch node := current.(type) {

	case map[string]interface{}:

		next, ok := node[keys[0]]

		if !ok {
			return nil, false
		}

		return getValueByPath(
			next,
			keys[1:],
		)

	case []interface{}:

		index, err := strconv.Atoi(keys[0])

		if err != nil ||
			index < 0 ||
			index >= len(node) {

			return nil, false
		}

		return getValueByPath(
			node[index],
			keys[1:],
		)
	}

	return nil, false
}

// ------------------------------------------------------------
// SET
// Schreibt Werte und baut Map-Pfade ggf. aus.
// ------------------------------------------------------------

func setValueByPath(
	current interface{},
	keys []string,
	value interface{},
) (interface{}, error) {

	if len(keys) == 0 {
		return value, nil
	}

	switch node := current.(type) {

	case map[string]interface{}:

		key := keys[0]

		if len(keys) == 1 {
			node[key] = value
			return node, nil
		}

		next, ok := node[key]

		if !ok {
			next = make(map[string]interface{})
		}

		updated, err := setValueByPath(
			next,
			keys[1:],
			value,
		)

		node[key] = updated

		return node, err

	case []interface{}:

		index, err := strconv.Atoi(keys[0])

		if err != nil ||
			index < 0 ||
			index >= len(node) {

			return nil, errors.New(
				"index out of range",
			)
		}

		if len(keys) == 1 {
			node[index] = value
			return node, nil
		}

		updated, err := setValueByPath(
			node[index],
			keys[1:],
			value,
		)

		node[index] = updated

		return node, err
	}

	return current, errors.New(
		"invalid path structure",
	)
}

// ------------------------------------------------------------
// splitPath
//
// Pfadsyntax:
//   "server.port"
//   "users.0.name"
//   "database.host"
// ------------------------------------------------------------

func splitPath(path string) []string {

	path = strings.TrimSpace(path)

	if path == "" {
		return nil
	}

	parts := strings.Split(path, ".")

	result := make([]string, 0, len(parts))

	for _, part := range parts {

		part = strings.TrimSpace(part)

		if part == "" {
			continue
		}

		result = append(result, part)
	}

	return result
}

// ------------------------------------------------------------
// main
// ------------------------------------------------------------

func main() {}
