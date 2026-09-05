package main

import (
	"encoding/json"
	"math"
	"unsafe"
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

// ------------------------------------------------------------
// ABI-Version
// ------------------------------------------------------------

//go:wasmexport vbx_abi_version
func vbxABIVersion() uint32 {
	return abiVersion
}

// ------------------------------------------------------------
// Wire-Format
//
// Muss exakt mit der Host-Seite übereinstimmen.
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

// ------------------------------------------------------------
// JSON-Ergebnisse
// ------------------------------------------------------------

func numResult(n float64) []byte {
	// JSON kennt weder NaN noch ±Inf.
	// Solche Ergebnisse dürfen daher nicht als "num"
	// zurückgegeben werden.
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return errorResult("Ergebnis ist keine gültige Zahl")
	}

	data, _ := json.Marshal(jsonValue{
		Type: "num",
		Num:  n,
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
// Hilfsfunktionen
// ------------------------------------------------------------

func requireNum(args []jsonValue, index int, function string) (float64, []byte) {
	if index >= len(args) {
		return 0, errorResult(
			function + ": Argument fehlt",
		)
	}

	if args[index].Type != "num" {
		return 0, errorResult(
			function + ": Argument " +
				itoa(index+1) +
				" muss eine Zahl sein",
		)
	}

	return args[index].Num, nil
}

// Kleine Integer-Konvertierung ohne zusätzliche Abhängigkeit.
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

// arrToFloats wandelt ein jsonValue vom Typ "arr" in []float64 um.
//
// Im Gegensatz zur bisherigen Version werden ungültige Elemente
// nicht stillschweigend übersprungen. Ein Array für finanzielle
// Berechnungen muss ausschließlich numerische Werte enthalten.
func arrToFloats(v jsonValue) ([]float64, bool) {
	if v.Type != "arr" {
		return nil, false
	}

	out := make([]float64, 0, len(v.Arr))

	for _, el := range v.Arr {
		if el.Type != "num" {
			return nil, false
		}

		out = append(out, el.Num)
	}

	return out, true
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {
	entries := []funcDesc{
		{
			Namespace:   "fin",
			Name:        "Fv",
			Params:      "rate, nper, pmt, [pv]",
			Description: "Berechnet den Endwert einer Investition.",
		},
		{
			Namespace:   "fin",
			Name:        "Pmt",
			Params:      "rate, nper, pv",
			Description: "Berechnet die periodische Rate eines Kredits.",
		},
		{
			Namespace:   "fin",
			Name:        "Npv",
			Params:      "rate, values",
			Description: "Berechnet den Kapitalwert (Net Present Value).",
		},
		{
			Namespace:   "fin",
			Name:        "Irr",
			Params:      "values, [guess]",
			Description: "Berechnet den Internen Zinsfuß.",
		},
		{
			Namespace:   "fin",
			Name:        "Fact",
			Params:      "n",
			Description: "Berechnet die Fakultät einer Ganzzahl (n!).",
		},
		{
			Namespace:   "fin",
			Name:        "Gamma",
			Params:      "n",
			Description: "Gibt den Wert der Gamma-Funktion zurück.",
		},
		{
			Namespace:   "fin",
			Name:        "Log10",
			Params:      "n",
			Description: "Berechnet den Zehnerlogarithmus.",
		},
		{
			Namespace:   "fin",
			Name:        "Log2",
			Params:      "n",
			Description: "Berechnet den Logarithmus zur Basis 2.",
		},
		{
			Namespace:   "fin",
			Name:        "Hypot",
			Params:      "x, y",
			Description: "Berechnet die Länge der Hypotenuse.",
		},
		{
			Namespace:   "fin",
			Name:        "Remainder",
			Params:      "x, y",
			Description: "Berechnet den Rest nach IEEE 754.",
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
		return packBytes(errorResult("ungültiger Funktionsname"))
	}

	if argsJSON == nil {
		return packBytes(errorResult("ungültige Argumente"))
	}

	name := string(nameBytes)

	var args []jsonValue

	if err := json.Unmarshal(argsJSON, &args); err != nil {
		return packBytes(
			errorResult("ungültige Argumente: " + err.Error()),
		)
	}

	switch name {
	case "Fv":
		return packBytes(handleFv(args))

	case "Pmt":
		return packBytes(handlePmt(args))

	case "Npv":
		return packBytes(handleNpv(args))

	case "Irr":
		return packBytes(handleIrr(args))

	case "Fact":
		return packBytes(handleFact(args))

	case "Gamma":
		return packBytes(handleGamma(args))

	case "Log10":
		return packBytes(handleLog10(args))

	case "Log2":
		return packBytes(handleLog2(args))

	case "Hypot":
		return packBytes(handleHypot(args))

	case "Remainder":
		return packBytes(handleRemainder(args))

	default:
		return packBytes(
			errorResult("unbekannte Funktion: " + name),
		)
	}
}

// ------------------------------------------------------------
// Fv
// ------------------------------------------------------------

func handleFv(args []jsonValue) []byte {
	if len(args) < 3 {
		return errorResult(
			"Fv erwartet mindestens 3 Argumente (rate, nper, pmt)",
		)
	}

	rate, err := requireNum(args, 0, "Fv")
	if err != nil {
		return err
	}

	nper, err := requireNum(args, 1, "Fv")
	if err != nil {
		return err
	}

	pmt, err := requireNum(args, 2, "Fv")
	if err != nil {
		return err
	}

	pv := 0.0

	if len(args) > 3 {
		pv, err = requireNum(args, 3, "Fv")
		if err != nil {
			return err
		}
	}

	if rate == 0 {
		return numResult(-(pv + pmt*nper))
	}

	term := math.Pow(1+rate, nper)

	return numResult(
		-(pv*term + pmt*(term-1)/rate),
	)
}

// ------------------------------------------------------------
// Pmt
// ------------------------------------------------------------

func handlePmt(args []jsonValue) []byte {
	if len(args) < 3 {
		return errorResult(
			"Pmt erwartet 3 Argumente (rate, nper, pv)",
		)
	}

	rate, err := requireNum(args, 0, "Pmt")
	if err != nil {
		return err
	}

	nper, err := requireNum(args, 1, "Pmt")
	if err != nil {
		return err
	}

	pv, err := requireNum(args, 2, "Pmt")
	if err != nil {
		return err
	}

	if rate == 0 {
		if nper == 0 {
			return errorResult("Pmt: nper darf nicht 0 sein")
		}

		return numResult(-pv / nper)
	}

	pv = -pv

	return numResult(
		(rate * pv) /
			(1 - math.Pow(1+rate, -nper)),
	)
}

// ------------------------------------------------------------
// Npv
// ------------------------------------------------------------

func handleNpv(args []jsonValue) []byte {
	if len(args) < 2 {
		return errorResult(
			"Npv erwartet 2 Argumente (rate, values)",
		)
	}

	rate, err := requireNum(args, 0, "Npv")
	if err != nil {
		return err
	}

	if args[1].Type != "arr" {
		return errorResult(
			"Npv: Zweites Argument muss ein Array sein",
		)
	}

	cashflows, ok := arrToFloats(args[1])
	if !ok {
		return errorResult(
			"Npv: Array muss ausschließlich Zahlen enthalten",
		)
	}

	npv := 0.0

	for i, value := range cashflows {
		npv += value /
			math.Pow(1+rate, float64(i+1))
	}

	return numResult(npv)
}

// ------------------------------------------------------------
// Irr
// ------------------------------------------------------------

func handleIrr(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult(
			"Irr erwartet mindestens 1 Argument (values)",
		)
	}

	if args[0].Type != "arr" {
		return errorResult(
			"Irr: Array erforderlich",
		)
	}

	cashflows, ok := arrToFloats(args[0])
	if !ok {
		return errorResult(
			"Irr: Array muss ausschließlich Zahlen enthalten",
		)
	}

	guess := 0.1

	if len(args) > 1 {
		var err []byte

		guess, err = requireNum(args, 1, "Irr")
		if err != nil {
			return err
		}
	}

	rate := guess

	for i := 0; i < 100; i++ {
		npv := 0.0
		dnpv := 0.0

		for t, value := range cashflows {
			tF := float64(t)

			npv += value /
				math.Pow(1+rate, tF)

			if t > 0 {
				dnpv -=
					tF * value /
						math.Pow(1+rate, tF+1)
			}
		}

		if math.IsNaN(npv) || math.IsInf(npv, 0) ||
			math.IsNaN(dnpv) || math.IsInf(dnpv, 0) {
			return errorResult(
				"Irr: Ungültiges Ergebnis",
			)
		}

		if math.Abs(npv) < 1e-7 {
			return numResult(rate)
		}

		if dnpv == 0 {
			break
		}

		newRate := rate - npv/dnpv

		if math.IsNaN(newRate) || math.IsInf(newRate, 0) {
			return errorResult(
				"Irr: Ungültiges Ergebnis",
			)
		}

		if math.Abs(newRate-rate) < 1e-7 {
			return numResult(newRate)
		}

		rate = newRate
	}

	return errorResult("Irr: Konvergiert nicht")
}

// ------------------------------------------------------------
// Fact
// ------------------------------------------------------------

func handleFact(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult(
			"Fact erwartet 1 Argument",
		)
	}

	value, err := requireNum(args, 0, "Fact")
	if err != nil {
		return err
	}

	n := int64(value)

	if float64(n) != value {
		return errorResult(
			"Fact: n muss eine Ganzzahl sein",
		)
	}

	if n < 0 {
		return errorResult(
			"Fakultät nicht für negative Zahlen",
		)
	}

	result := 1.0

	for i := int64(2); i <= n; i++ {
		result *= float64(i)

		if math.IsInf(result, 0) {
			return errorResult(
				"Fact: Ergebnis ist zu groß",
			)
		}
	}

	return numResult(result)
}

// ------------------------------------------------------------
// Gamma
// ------------------------------------------------------------

func handleGamma(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult(
			"Gamma erwartet 1 Argument",
		)
	}

	n, err := requireNum(args, 0, "Gamma")
	if err != nil {
		return err
	}

	return numResult(
		math.Gamma(n),
	)
}

// ------------------------------------------------------------
// Log10
// ------------------------------------------------------------

func handleLog10(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult(
			"Log10 erwartet 1 Argument",
		)
	}

	n, err := requireNum(args, 0, "Log10")
	if err != nil {
		return err
	}

	if n <= 0 {
		return errorResult(
			"Log10: Argument muss größer als 0 sein",
		)
	}

	return numResult(
		math.Log10(n),
	)
}

// ------------------------------------------------------------
// Log2
// ------------------------------------------------------------

func handleLog2(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult(
			"Log2 erwartet 1 Argument",
		)
	}

	n, err := requireNum(args, 0, "Log2")
	if err != nil {
		return err
	}

	if n <= 0 {
		return errorResult(
			"Log2: Argument muss größer als 0 sein",
		)
	}

	return numResult(
		math.Log2(n),
	)
}

// ------------------------------------------------------------
// Hypot
// ------------------------------------------------------------

func handleHypot(args []jsonValue) []byte {
	if len(args) < 2 {
		return errorResult(
			"Hypot erwartet 2 Argumente",
		)
	}

	x, err := requireNum(args, 0, "Hypot")
	if err != nil {
		return err
	}

	y, err := requireNum(args, 1, "Hypot")
	if err != nil {
		return err
	}

	return numResult(
		math.Hypot(x, y),
	)
}

// ------------------------------------------------------------
// Remainder
// ------------------------------------------------------------

func handleRemainder(args []jsonValue) []byte {
	if len(args) < 2 {
		return errorResult(
			"Remainder erwartet 2 Argumente",
		)
	}

	x, err := requireNum(args, 0, "Remainder")
	if err != nil {
		return err
	}

	y, err := requireNum(args, 1, "Remainder")
	if err != nil {
		return err
	}

	if y == 0 {
		return errorResult(
			"Remainder: Divisor darf nicht 0 sein",
		)
	}

	return numResult(
		math.Remainder(x, y),
	)
}

// ------------------------------------------------------------
// Reactor entry point
//
// Bei normalem Go + -buildmode=c-shared wird daraus
// der WASI-Reactor mit _initialize.
// ------------------------------------------------------------

func main() {}
