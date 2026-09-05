package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"unsafe"
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
// INI Store
// ============================================================

type iniStore struct {
	mu   sync.RWMutex
	data map[string]map[string]string
	file string
}

var defaultIni = &iniStore{
	data: make(map[string]map[string]string),
}

// ------------------------------------------------------------
// INI speichern
// ------------------------------------------------------------

func (s *iniStore) save() error {
	if s.file == "" {
		return fmt.Errorf(
			"keine INI-Datei geladen (ini.Load zuerst aufrufen)",
		)
	}

	var sb strings.Builder

	sections := make([]string, 0, len(s.data))
	for sec := range s.data {
		sections = append(sections, sec)
	}
	sort.Strings(sections)

	for _, sec := range sections {
		sb.WriteByte('[')
		sb.WriteString(sec)
		sb.WriteString("]\n")

		keys := make([]string, 0, len(s.data[sec]))
		for key := range s.data[sec] {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			sb.WriteString(key)
			sb.WriteByte('=')
			sb.WriteString(s.data[sec][key])
			sb.WriteByte('\n')
		}

		sb.WriteByte('\n')
	}

	tmp := s.file + ".tmp"

	if err := os.WriteFile(
		tmp,
		[]byte(sb.String()),
		0644,
	); err != nil {
		return fmt.Errorf(
			"temporäre Datei konnte nicht geschrieben werden: %w",
			err,
		)
	}

	if err := os.Rename(tmp, s.file); err != nil {
		_ = os.Remove(tmp)

		return fmt.Errorf(
			"atomares Ersetzen fehlgeschlagen: %w",
			err,
		)
	}

	return nil
}

// ------------------------------------------------------------
// INI laden
// ------------------------------------------------------------

func (s *iniStore) load(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.file = path
	s.data = make(map[string]map[string]string)

	data, err := os.ReadFile(path)

	if err != nil {
		if os.IsNotExist(err) {
			// Nicht vorhandene Datei ist ein gültiger,
			// leerer INI-Zustand.
			return nil
		}

		return fmt.Errorf(
			"INI-Lesefehler: %w",
			err,
		)
	}

	var currentSection string

	for _, line := range strings.Split(
		string(data),
		"\n",
	) {
		line = strings.TrimSpace(line)

		// Leerzeilen und Kommentare
		if line == "" ||
			strings.HasPrefix(line, ";") ||
			strings.HasPrefix(line, "#") {
			continue
		}

		// Sektion
		if strings.HasPrefix(line, "[") &&
			strings.HasSuffix(line, "]") {

			currentSection = strings.TrimSpace(
				line[1 : len(line)-1],
			)

			if s.data[currentSection] == nil {
				s.data[currentSection] =
					make(map[string]string)
			}

			continue
		}

		// Einträge außerhalb einer Sektion ignorieren.
		if currentSection == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)

		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		s.data[currentSection][key] = value
	}

	return nil
}

// ------------------------------------------------------------
// INI Get
// ------------------------------------------------------------

func (s *iniStore) get(
	section string,
	key string,
	def string,
) string {

	s.mu.RLock()
	defer s.mu.RUnlock()

	if sec, ok := s.data[section]; ok {
		if value, ok := sec[key]; ok {
			return value
		}
	}

	return def
}

// ------------------------------------------------------------
// INI Set
// ------------------------------------------------------------

func (s *iniStore) set(
	section string,
	key string,
	value string,
	autosave bool,
) error {

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data[section] == nil {
		s.data[section] =
			make(map[string]string)
	}

	s.data[section][key] = value

	if autosave {
		return s.save()
	}

	return nil
}

// ------------------------------------------------------------
// INI Exists
// ------------------------------------------------------------

func (s *iniStore) exists(
	section string,
	key string,
) bool {

	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.data[section][key]

	return ok
}

// ------------------------------------------------------------
// INI Sections
// ------------------------------------------------------------

func (s *iniStore) sections() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.data))

	for section := range s.data {
		result = append(result, section)
	}

	sort.Strings(result)

	return result
}

// ------------------------------------------------------------
// INI Keys
// ------------------------------------------------------------

func (s *iniStore) keys(section string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sec := s.data[section]

	result := make([]string, 0, len(sec))

	for key := range sec {
		result = append(result, key)
	}

	sort.Strings(result)

	return result
}

// ------------------------------------------------------------
// INI Delete
// ------------------------------------------------------------

func (s *iniStore) delete(
	section string,
	key *string,
) error {

	s.mu.Lock()
	defer s.mu.Unlock()

	if key == nil {
		delete(s.data, section)
		return s.save()
	}

	if sec, ok := s.data[section]; ok {
		if _, exists := sec[*key]; exists {
			delete(sec, *key)

			if len(sec) == 0 {
				delete(s.data, section)
			}

			return s.save()
		}
	}

	return nil
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

func makeArray(values []string) jsonValue {
	arr := make([]jsonValue, len(values))

	for i, value := range values {
		arr[i] = makeString(value)
	}

	return jsonValue{
		Type: "arr",
		Arr:  arr,
	}
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

func argBool(
	args []jsonValue,
	index int,
	defaultValue bool,
) bool {

	if index >= len(args) {
		return defaultValue
	}

	switch args[index].Type {
	case "bool":
		return args[index].Bool

	case "num":
		return args[index].Num != 0

	case "str":
		switch strings.ToLower(
			strings.TrimSpace(args[index].Str),
		) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}

	return defaultValue
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
	// ini.Load
	// --------------------------------------------------------

	case "Load":

		path, err := argString(args, 0)
		if err != nil {
			return makeError(err)
		}

		if err := defaultIni.load(path); err != nil {
			return makeError(err)
		}

		return makeEmpty()

	// --------------------------------------------------------
	// ini.Get
	// --------------------------------------------------------

	case "Get":

		section, err := argString(args, 0)
		if err != nil {
			return makeError(err)
		}

		key, err := argString(args, 1)
		if err != nil {
			return makeError(err)
		}

		def := ""

		if len(args) >= 3 {
			def, err = argString(args, 2)
			if err != nil {
				return makeError(err)
			}
		}

		return makeString(
			defaultIni.get(
				section,
				key,
				def,
			),
		)

	// --------------------------------------------------------
	// ini.Set
	// --------------------------------------------------------

	case "Set":

		section, err := argString(args, 0)
		if err != nil {
			return makeError(err)
		}

		key, err := argString(args, 1)
		if err != nil {
			return makeError(err)
		}

		if len(args) < 3 {
			return makeError(
				fmt.Errorf(
					"ini.Set(section, key, value [, autosave]) benötigt mindestens 3 Argumente",
				),
			)
		}

		value := ""

		switch args[2].Type {

		case "str":
			value = args[2].Str

		case "num":
			value = fmt.Sprintf(
				"%v",
				args[2].Num,
			)

		case "bool":
			if args[2].Bool {
				value = "true"
			} else {
				value = "false"
			}

		default:
			return makeError(
				fmt.Errorf(
					"ini.Set: value muss String, Zahl oder Bool sein",
				),
			)
		}

		autosave := argBool(
			args,
			3,
			true,
		)

		if err := defaultIni.set(
			section,
			key,
			value,
			autosave,
		); err != nil {
			return makeError(err)
		}

		return makeEmpty()

	// --------------------------------------------------------
	// ini.Save
	// --------------------------------------------------------

	case "Save":

		defaultIni.mu.Lock()
		err := defaultIni.save()
		defaultIni.mu.Unlock()

		if err != nil {
			return makeError(err)
		}

		return makeEmpty()

	// --------------------------------------------------------
	// ini.Exists
	// --------------------------------------------------------

	case "Exists":

		section, err := argString(args, 0)
		if err != nil {
			return makeError(err)
		}

		key, err := argString(args, 1)
		if err != nil {
			return makeError(err)
		}

		return makeBool(
			defaultIni.exists(
				section,
				key,
			),
		)

	// --------------------------------------------------------
	// ini.Sections
	// --------------------------------------------------------

	case "Sections":

		return makeArray(
			defaultIni.sections(),
		)

	// --------------------------------------------------------
	// ini.Keys
	// --------------------------------------------------------

	case "Keys":

		section, err := argString(args, 0)
		if err != nil {
			return makeError(err)
		}

		return makeArray(
			defaultIni.keys(section),
		)

	// --------------------------------------------------------
	// ini.Delete
	// --------------------------------------------------------

	case "Delete":

		section, err := argString(args, 0)
		if err != nil {
			return makeError(err)
		}

		var key *string

		if len(args) >= 2 {
			value, err := argString(args, 1)
			if err != nil {
				return makeError(err)
			}

			key = &value
		}

		if err := defaultIni.delete(
			section,
			key,
		); err != nil {
			return makeError(err)
		}

		return makeEmpty()

	default:

		return makeError(
			fmt.Errorf(
				"unbekannte Funktion: ini.%s",
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
			Namespace:   "ini",
			Name:        "Load",
			Params:      "filename",
			Description: "Lädt eine INI-Datei in den Speicher.",
		},

		{
			Namespace:   "ini",
			Name:        "Get",
			Params:      "section, key [, default]",
			Description: "Liest einen Wert aus einer Sektion.",
		},

		{
			Namespace:   "ini",
			Name:        "Set",
			Params:      "section, key, value [, autosave]",
			Description: "Setzt einen Wert in einer Sektion.",
		},

		{
			Namespace:   "ini",
			Name:        "Save",
			Params:      "",
			Description: "Speichert die aktuellen Änderungen in die INI-Datei.",
		},

		{
			Namespace:   "ini",
			Name:        "Exists",
			Params:      "section, key",
			Description: "Prüft, ob ein Key in einer Sektion existiert.",
		},

		{
			Namespace:   "ini",
			Name:        "Sections",
			Params:      "",
			Description: "Gibt alle Sektionsnamen als Array zurück.",
		},

		{
			Namespace:   "ini",
			Name:        "Keys",
			Params:      "section",
			Description: "Gibt alle Keys einer Sektion als Array zurück.",
		},

		{
			Namespace:   "ini",
			Name:        "Delete",
			Params:      "section [, key]",
			Description: "Löscht einen Key oder eine komplette Sektion.",
		},
	}

	descriptionJSON, _ = json.Marshal(functions)
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

	data, err := json.Marshal(value)

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
