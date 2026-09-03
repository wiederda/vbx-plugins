package main

import (
	"encoding/json"
	"fmt"
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
	Type    string  `json:"type"`
	Num     float64 `json:"num,omitempty"`
	Str     string  `json:"str,omitempty"`
	Bool    bool    `json:"bool,omitempty"`
	Message string  `json:"message,omitempty"`
}

type funcDesc struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Params      string `json:"params"`
	Description string `json:"description"`
}

// ------------------------------------------------------------
// Speicher / JSON
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

func errorResult(msg string) []byte {
	data, _ := json.Marshal(jsonValue{
		Type:    "error",
		Message: msg,
	})
	return data
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//export vbx_describe
func vbx_describe() uint64 {
	entries := []funcDesc{

		// Datenmengen - SI
		{
			Namespace:   "data",
			Name:        "ByteToKb",
			Params:      "val",
			Description: "Konvertiert Byte in Kilobyte (1000).",
		},
		{
			Namespace:   "data",
			Name:        "KbToMb",
			Params:      "val",
			Description: "Konvertiert Kilobyte in Megabyte (1000).",
		},
		{
			Namespace:   "data",
			Name:        "MbToGb",
			Params:      "val",
			Description: "Konvertiert Megabyte in Gigabyte (1000).",
		},
		{
			Namespace:   "data",
			Name:        "GbToTb",
			Params:      "val",
			Description: "Konvertiert Gigabyte in Terabyte (1000).",
		},

		// Datenmengen - Binär
		{
			Namespace:   "data",
			Name:        "ByteToKiB",
			Params:      "val",
			Description: "Konvertiert Byte in Kibibyte (1024).",
		},
		{
			Namespace:   "data",
			Name:        "KiBToMiB",
			Params:      "val",
			Description: "Konvertiert Kibibyte in Mebibyte (1024).",
		},
		{
			Namespace:   "data",
			Name:        "MiBToGiB",
			Params:      "val",
			Description: "Konvertiert Mebibyte in Gibibyte (1024).",
		},
		{
			Namespace:   "data",
			Name:        "GiBToTiB",
			Params:      "val",
			Description: "Konvertiert Gibibyte in Tebibyte (1024).",
		},

		// Leistung
		{
			Namespace:   "data",
			Name:        "WattToKilowatt",
			Params:      "val",
			Description: "Konvertiert Watt in Kilowatt.",
		},
		{
			Namespace:   "data",
			Name:        "KilowattToWatt",
			Params:      "val",
			Description: "Konvertiert Kilowatt in Watt.",
		},

		// Zeit
		{
			Namespace:   "data",
			Name:        "MinutesToHours",
			Params:      "val",
			Description: "Konvertiert Minuten in Stunden.",
		},
		{
			Namespace:   "data",
			Name:        "HoursToMinutes",
			Params:      "val",
			Description: "Konvertiert Stunden in Minuten.",
		},
		{
			Namespace:   "data",
			Name:        "SecondsToDays",
			Params:      "val",
			Description: "Konvertiert Sekunden in Tage.",
		},
		{
			Namespace:   "data",
			Name:        "DaysToSeconds",
			Params:      "val",
			Description: "Konvertiert Tage in Sekunden.",
		},

		// Formatierung
		{
			Namespace:   "data",
			Name:        "FormatSeconds",
			Params:      "seconds",
			Description: "Formatiert Sekunden als HH:MM:SS.",
		},
	}

	data, _ := json.Marshal(entries)
	return packBytes(data)
}

// ------------------------------------------------------------
// vbx_call
// ------------------------------------------------------------

//export vbx_call
func vbx_call(namePtr, nameLen, argsPtr, argsLen uint32) uint64 {
	name := string(readBytes(namePtr, nameLen))
	argsJSON := readBytes(argsPtr, argsLen)

	var args []jsonValue

	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return packBytes(errorResult(
			"ungültige Argumente: " + err.Error(),
		))
	}

	switch name {
	case "ByteToKb":
		return packBytes(handleByteToKb(args))

	case "KbToMb":
		return packBytes(handleKbToMb(args))

	case "MbToGb":
		return packBytes(handleMbToGb(args))

	case "GbToTb":
		return packBytes(handleGbToTb(args))

	case "ByteToKiB":
		return packBytes(handleByteToKiB(args))

	case "KiBToMiB":
		return packBytes(handleKiBToMiB(args))

	case "MiBToGiB":
		return packBytes(handleMiBToGiB(args))

	case "GiBToTiB":
		return packBytes(handleGiBToTiB(args))

	case "WattToKilowatt":
		return packBytes(handleWattToKilowatt(args))

	case "KilowattToWatt":
		return packBytes(handleKilowattToWatt(args))

	case "MinutesToHours":
		return packBytes(handleMinutesToHours(args))

	case "HoursToMinutes":
		return packBytes(handleHoursToMinutes(args))

	case "SecondsToDays":
		return packBytes(handleSecondsToDays(args))

	case "DaysToSeconds":
		return packBytes(handleDaysToSeconds(args))

	case "FormatSeconds":
		return packBytes(handleFormatSeconds(args))

	default:
		return packBytes(errorResult(
			"unbekannte Funktion: " + name,
		))
	}
}

// ------------------------------------------------------------
// Hilfsfunktion
// ------------------------------------------------------------

func firstNumber(args []jsonValue) (float64, bool) {
	if len(args) < 1 || args[0].Type != "num" {
		return 0, false
	}

	return args[0].Num, true
}

// ------------------------------------------------------------
// Datenmengen - SI
// ------------------------------------------------------------

func handleByteToKb(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("ByteToKb erwartet 1 numerisches Argument")
	}

	return numResult(v / 1000)
}

func handleKbToMb(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("KbToMb erwartet 1 numerisches Argument")
	}

	return numResult(v / 1000)
}

func handleMbToGb(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("MbToGb erwartet 1 numerisches Argument")
	}

	return numResult(v / 1000)
}

func handleGbToTb(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("GbToTb erwartet 1 numerisches Argument")
	}

	return numResult(v / 1000)
}

// ------------------------------------------------------------
// Datenmengen - Binär
// ------------------------------------------------------------

func handleByteToKiB(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("ByteToKiB erwartet 1 numerisches Argument")
	}

	return numResult(v / 1024)
}

func handleKiBToMiB(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("KiBToMiB erwartet 1 numerisches Argument")
	}

	return numResult(v / 1024)
}

func handleMiBToGiB(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("MiBToGiB erwartet 1 numerisches Argument")
	}

	return numResult(v / 1024)
}

func handleGiBToTiB(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("GiBToTiB erwartet 1 numerisches Argument")
	}

	return numResult(v / 1024)
}

// ------------------------------------------------------------
// Leistung
// ------------------------------------------------------------

func handleWattToKilowatt(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("WattToKilowatt erwartet 1 numerisches Argument")
	}

	return numResult(v / 1000)
}

func handleKilowattToWatt(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("KilowattToWatt erwartet 1 numerisches Argument")
	}

	return numResult(v * 1000)
}

// ------------------------------------------------------------
// Zeit
// ------------------------------------------------------------

func handleMinutesToHours(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("MinutesToHours erwartet 1 numerisches Argument")
	}

	return numResult(v / 60)
}

func handleHoursToMinutes(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("HoursToMinutes erwartet 1 numerisches Argument")
	}

	return numResult(v * 60)
}

func handleSecondsToDays(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("SecondsToDays erwartet 1 numerisches Argument")
	}

	return numResult(v / 86400)
}

func handleDaysToSeconds(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("DaysToSeconds erwartet 1 numerisches Argument")
	}

	return numResult(v * 86400)
}

// ------------------------------------------------------------
// Formatierung
// ------------------------------------------------------------

func handleFormatSeconds(args []jsonValue) []byte {
	v, ok := firstNumber(args)
	if !ok {
		return errorResult("FormatSeconds erwartet 1 numerisches Argument")
	}

	s := int(v)

	h := s / 3600
	m := (s % 3600) / 60
	sec := s % 60

	return strResult(fmt.Sprintf(
		"%02d:%02d:%02d",
		h, m, sec,
	))
}

// ------------------------------------------------------------

func main() {}
