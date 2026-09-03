package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
	"unsafe"
)

// ------------------------------------------------------------
// Speicherverwaltung
// ------------------------------------------------------------

var liveBuffers = map[uint32][]byte{}

var randGen = rand.New(rand.NewSource(time.Now().UnixNano()))

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
// Hilfsfunktionen
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

func numResult(n float64) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "num",
		Num:  n,
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
// vbx_describe
// ------------------------------------------------------------

//export vbx_describe
func vbx_describe() uint64 {
	entries := []funcDesc{
		{
			Namespace:   "rand",
			Name:        "Float",
			Params:      "-",
			Description: "Gibt eine Zufallszahl zwischen 0.0 und 1.0 zurück.",
		},
		{
			Namespace:   "rand",
			Name:        "Choice",
			Params:      "array",
			Description: "Wählt ein zufälliges Element aus einem Array aus.",
		},
		{
			Namespace:   "rand",
			Name:        "Bool",
			Params:      "-",
			Description: "Gibt zufällig true oder false zurück.",
		},
		{
			Namespace:   "rand",
			Name:        "Range",
			Params:      "min, max",
			Description: "Gibt eine Ganzzahl zwischen min und max inklusive zurück.",
		},
		{
			Namespace:   "rand",
			Name:        "RangeFloat",
			Params:      "min, max",
			Description: "Gibt eine Fließkommazahl im angegebenen Bereich zurück.",
		},
		{
			Namespace:   "rand",
			Name:        "Seed",
			Params:      "[n]",
			Description: "Initialisiert den Zufallsgenerator mit einem Startwert.",
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
		return packBytes(
			errorResult(
				"ungültige Argumente: " + err.Error(),
			),
		)
	}

	switch name {

	case "Float":
		return packBytes(handleFloat(args))

	case "Choice":
		return packBytes(handleChoice(args))

	case "Bool":
		return packBytes(handleBool(args))

	case "Range":
		return packBytes(handleRange(args))

	case "RangeFloat":
		return packBytes(handleRangeFloat(args))

	case "Seed":
		return packBytes(handleSeed(args))

	default:
		return packBytes(
			errorResult(
				"unbekannte Funktion: " + name,
			),
		)
	}
}

// ------------------------------------------------------------
// rand.Float()
// ------------------------------------------------------------

func handleFloat(args []jsonValue) []byte {
	return numResult(randGen.Float64())
}

// ------------------------------------------------------------
// rand.Choice(array)
// ------------------------------------------------------------

func handleChoice(args []jsonValue) []byte {

	if len(args) < 1 {
		return errorResult(
			"rand.Choice erwartet ein Array",
		)
	}

	if args[0].Type != "arr" {
		return errorResult(
			"rand.Choice erwartet ein Array",
		)
	}

	arr := args[0].Arr

	if len(arr) == 0 {
		return errorResult(
			"Array ist leer",
		)
	}

	idx := randGen.Intn(len(arr))

	return valueResult(arr[idx])
}

// ------------------------------------------------------------
// rand.Bool()
// ------------------------------------------------------------

func handleBool(args []jsonValue) []byte {
	return boolResult(randGen.Intn(2) == 0)
}

// ------------------------------------------------------------
// rand.Range(min, max)
// ------------------------------------------------------------

func handleRange(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult(
			"rand.Range erwartet mindestens 2 Argumente (min, max)",
		)
	}

	if args[0].Type != "num" {
		return errorResult(
			"rand.Range: min muss eine Zahl sein",
		)
	}

	if args[1].Type != "num" {
		return errorResult(
			"rand.Range: max muss eine Zahl sein",
		)
	}

	minV := args[0].Num
	maxV := args[1].Num

	iMin := int(minV)
	iMax := int(maxV)

	if iMax < iMin {
		return errorResult(
			fmt.Sprintf(
				"rand.Range: max (%d) darf nicht kleiner als min (%d) sein",
				iMax,
				iMin,
			),
		)
	}

	if iMax == iMin {
		return numResult(float64(iMin))
	}

	return numResult(
		float64(
			randGen.Intn(iMax-iMin+1) + iMin,
		),
	)
}

// ------------------------------------------------------------
// rand.RangeFloat(min, max)
// ------------------------------------------------------------

func handleRangeFloat(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult(
			"rand.RangeFloat erwartet mindestens 2 Argumente (min, max)",
		)
	}

	if args[0].Type != "num" {
		return errorResult(
			"rand.RangeFloat: min muss eine Zahl sein",
		)
	}

	if args[1].Type != "num" {
		return errorResult(
			"rand.RangeFloat: max muss eine Zahl sein",
		)
	}

	minV := args[0].Num
	maxV := args[1].Num

	if maxV < minV {
		return errorResult(
			fmt.Sprintf(
				"rand.RangeFloat: max (%v) darf nicht kleiner als min (%v) sein",
				maxV,
				minV,
			),
		)
	}

	if maxV == minV {
		return numResult(minV)
	}

	return numResult(
		minV + randGen.Float64()*(maxV-minV),
	)
}

// ------------------------------------------------------------
// rand.Seed([n])
// ------------------------------------------------------------

func handleSeed(args []jsonValue) []byte {

	// Kein Argument:
	// Zufallsgenerator mit aktueller Zeit neu initialisieren.
	if len(args) == 0 {

		randGen = rand.New(
			rand.NewSource(
				time.Now().UnixNano(),
			),
		)

		return numResult(0)
	}

	if args[0].Type != "num" {
		return errorResult(
			"rand.Seed: n muss eine Zahl sein",
		)
	}

	seed := args[0].Num

	randGen = rand.New(
		rand.NewSource(
			int64(seed),
		),
	)

	return numResult(seed)
}

// ------------------------------------------------------------
// main
// ------------------------------------------------------------

func main() {}