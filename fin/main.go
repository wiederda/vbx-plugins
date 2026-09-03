package main

import (
	"encoding/json"
	"math"
	"unsafe"
)

// ------------------------------------------------------------
// Speicherverwaltung (identisch zum demo-Plugin)
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
// Wire-Format - MUSS mit der Host-Seite übereinstimmen
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
	data, _ := json.Marshal(jsonValue{Type: "num", Num: n})
	return data
}

func errorResult(msg string) []byte {
	data, _ := json.Marshal(jsonValue{Type: "error", Message: msg})
	return data
}

// arrToFloats wandelt ein jsonValue vom Typ "arr" in []float64 um,
// nicht-numerische Elemente werden übersprungen (analog zur
// Host-Implementierung, die ebenfalls nur val.Kind == KindNum prüft).
func arrToFloats(v jsonValue) []float64 {
	out := make([]float64, 0, len(v.Arr))
	for _, el := range v.Arr {
		if el.Type == "num" {
			out = append(out, el.Num)
		}
	}
	return out
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//export vbx_describe
func vbx_describe() uint64 {
	entries := []funcDesc{
		{Namespace: "fin", Name: "Fv", Params: "rate, nper, pmt, [pv]", Description: "Berechnet den Endwert einer Investition."},
		{Namespace: "fin", Name: "Pmt", Params: "rate, nper, pv", Description: "Berechnet die periodische Rate eines Kredits."},
		{Namespace: "fin", Name: "Npv", Params: "rate, values", Description: "Berechnet den Kapitalwert (Net Present Value)."},
		{Namespace: "fin", Name: "Irr", Params: "values, [guess]", Description: "Berechnet den Internen Zinsfuß."},
		{Namespace: "fin", Name: "Fact", Params: "n", Description: "Berechnet die Fakultät einer Ganzzahl (n!)."},
		{Namespace: "fin", Name: "Gamma", Params: "n", Description: "Gibt den Wert der Gamma-Funktion zurück."},
		{Namespace: "fin", Name: "Log10", Params: "n", Description: "Berechnet den Zehnerlogarithmus."},
		{Namespace: "fin", Name: "Log2", Params: "n", Description: "Berechnet den Logarithmus zur Basis 2."},
		{Namespace: "fin", Name: "Hypot", Params: "x, y", Description: "Berechnet die Länge der Hypotenuse."},
		{Namespace: "fin", Name: "Remainder", Params: "x, y", Description: "Berechnet den Rest nach IEEE 754."},
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
		return packBytes(errorResult("ungültige Argumente: " + err.Error()))
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
		return packBytes(errorResult("unbekannte Funktion: " + name))
	}
}

// ------------------------------------------------------------
// Handler - 1:1 Logik aus stdlib_fin.go
// ------------------------------------------------------------

func handleFv(args []jsonValue) []byte {
	if len(args) < 3 {
		return errorResult("Fv erwartet mindestens 3 Argumente (rate, nper, pmt)")
	}
	rate, nper, pmt := args[0].Num, args[1].Num, args[2].Num
	pv := 0.0
	if len(args) > 3 {
		pv = args[3].Num
	}
	if rate == 0 {
		return numResult(-(pv + pmt*nper))
	}
	term := math.Pow(1+rate, nper)
	return numResult(-(pv*term + pmt*(term-1)/rate))
}

func handlePmt(args []jsonValue) []byte {
	if len(args) < 3 {
		return errorResult("Pmt erwartet 3 Argumente (rate, nper, pv)")
	}
	rate, nper, pv := args[0].Num, args[1].Num, args[2].Num
	if rate == 0 {
		return numResult(-pv / nper)
	}
	pv = -pv
	return numResult((rate * pv) / (1 - math.Pow(1+rate, -nper)))
}

func handleNpv(args []jsonValue) []byte {
	if len(args) < 2 || args[1].Type != "arr" {
		return errorResult("Npv: Zweites Argument muss ein Array sein")
	}
	rate := args[0].Num
	cashflows := arrToFloats(args[1])
	npv := 0.0
	for i, v := range cashflows {
		npv += v / math.Pow(1+rate, float64(i+1))
	}
	return numResult(npv)
}

func handleIrr(args []jsonValue) []byte {
	if len(args) < 1 || args[0].Type != "arr" {
		return errorResult("Irr: Array erforderlich")
	}
	cashflows := arrToFloats(args[0])
	guess := 0.1
	if len(args) > 1 && args[1].Type == "num" {
		guess = args[1].Num
	}

	rate := guess
	for i := 0; i < 100; i++ {
		npv, dnpv := 0.0, 0.0
		for t, v := range cashflows {
			tF := float64(t)
			npv += v / math.Pow(1+rate, tF)
			if t > 0 {
				dnpv -= tF * v / math.Pow(1+rate, tF+1)
			}
		}
		if math.Abs(npv) < 1e-7 {
			return numResult(rate)
		}
		if dnpv == 0 {
			break
		}
		newRate := rate - npv/dnpv
		if math.Abs(newRate-rate) < 1e-7 {
			return numResult(newRate)
		}
		rate = newRate
	}
	return errorResult("Irr: Konvergiert nicht")
}

func handleFact(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult("Fact erwartet 1 Argument")
	}
	n := int64(args[0].Num)
	if n < 0 {
		return errorResult("Fakultät nicht für negative Zahlen")
	}
	res := 1.0
	for i := int64(2); i <= n; i++ {
		res *= float64(i)
	}
	return numResult(res)
}

func handleGamma(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult("Gamma erwartet 1 Argument")
	}
	return numResult(math.Gamma(args[0].Num))
}

func handleLog10(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult("Log10 erwartet 1 Argument")
	}
	return numResult(math.Log10(args[0].Num))
}

func handleLog2(args []jsonValue) []byte {
	if len(args) < 1 {
		return errorResult("Log2 erwartet 1 Argument")
	}
	return numResult(math.Log2(args[0].Num))
}

func handleHypot(args []jsonValue) []byte {
	if len(args) < 2 {
		return errorResult("Hypot erwartet 2 Argumente")
	}
	return numResult(math.Hypot(args[0].Num, args[1].Num))
}

func handleRemainder(args []jsonValue) []byte {
	if len(args) < 2 {
		return errorResult("Remainder erwartet 2 Argumente")
	}
	return numResult(math.Remainder(args[0].Num, args[1].Num))
}

func main() {}