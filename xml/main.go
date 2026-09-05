package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unsafe"
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
	Type    string               `json:"type"`
	Num     float64              `json:"num,omitempty"`
	Str     string               `json:"str,omitempty"`
	Bool    bool                 `json:"bool,omitempty"`
	Arr     []jsonValue          `json:"arr,omitempty"`
	Map     map[string]jsonValue `json:"map,omitempty"`
	Message string               `json:"message,omitempty"`
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

func boolResult(value bool) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "bool",
		Bool: value,
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

func numResult(value float64) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "num",
		Num:  value,
	})

	return data
}

func arrayResult(values []jsonValue) []byte {
	data, _ := json.Marshal(jsonValue{
		Type: "arr",
		Arr:  values,
	})

	return data
}

func valueResult(v jsonValue) []byte {
	data, _ := json.Marshal(v)
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

// ------------------------------------------------------------
// XML-Engine
// ------------------------------------------------------------
//
// Hinweis zur Statushaltung:
// xmlEngine ist wie in der nativen Version ein einzelner globaler
// Singleton innerhalb DIESES Plugin-Moduls. Da wazero-Modulinstanzen
// über die gesamte Prozesslaufzeit geladen bleiben (siehe
// loadedPlugins in LoadWasmPlugin), verhält sich das exakt wie die
// native Variante: ein geladenes Dokument bleibt zwischen mehreren
// vbx_call-Aufrufen bestehen, bis erneut xml.Load aufgerufen wird.
//
// Kein Mutex nötig: der Host serialisiert Aufrufe in dieselbe
// Modulinstanz ohnehin (parallele Calls in dieselbe WASM-Instanz
// sind bei wazero nicht vorgesehen) - das gilt für liveBuffers/alloc
// genauso wie hier, also konsistent mit den anderen Plugins.
//

type XmlEngine struct {
	Root *Node
	Path string
}

type Node struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Content string     `xml:",chardata"`
	Nodes   []*Node    `xml:",any"`
}

var xmlEngine = &XmlEngine{}

func splitPath(path string) []string {
	if path == "" {
		return []string{}
	}
	return strings.Split(path, ".")
}

func parseStep(step string) (string, int) {
	if idxStart := strings.Index(step, "["); idxStart != -1 {
		idxEnd := strings.Index(step, "]")
		if idxEnd != -1 {
			name := step[:idxStart]
			idx, err := strconv.Atoi(step[idxStart+1 : idxEnd])
			if err == nil {
				return name, idx
			}
		}
	}
	return step, 0
}

func (x *XmlEngine) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		x.Root = nil
		x.Path = path
		return err
	}

	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var stack []*Node
	var root *Node

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("XML Parse-Fehler: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			node := &Node{XMLName: t.Name, Attrs: t.Attr}
			if len(stack) == 0 {
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.Nodes = append(parent.Nodes, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				text := strings.TrimSpace(string(t))
				if text != "" {
					stack[len(stack)-1].Content = text
				}
			}
		}
	}

	x.Root = root
	x.Path = path
	return nil
}

func (x *XmlEngine) find(path string) (*Node, *Node, error) {
	parts := splitPath(path)
	if x.Root == nil || len(parts) == 0 {
		return nil, nil, errors.New("not found")
	}

	rootName, rootIdx := parseStep(parts[0])

	if x.Root.XMLName.Local != rootName || rootIdx != 0 {
		return nil, nil, errors.New("root mismatch")
	}

	current := x.Root
	var parent *Node

	for _, p := range parts[1:] {
		tagName, targetIdx := parseStep(p)
		found := false
		count := 0

		for _, n := range current.Nodes {
			if n.XMLName.Local == tagName {
				if count == targetIdx {
					parent = current
					current = n
					found = true
					break
				}
				count++
			}
		}
		if !found {
			return nil, nil, fmt.Errorf("path not found: %s", p)
		}
	}
	return current, parent, nil
}

func (x *XmlEngine) ensurePath(path string) error {
	parts := splitPath(path)
	if len(parts) == 0 {
		return errors.New("invalid path")
	}

	rootName, _ := parseStep(parts[0])
	if x.Root == nil {
		x.Root = &Node{XMLName: xml.Name{Local: rootName}}
	}
	if x.Root.XMLName.Local != rootName {
		return errors.New("root mismatch")
	}

	current := x.Root
	for _, p := range parts[1:] {
		tagName, targetIdx := parseStep(p)
		var next *Node
		count := 0

		for _, n := range current.Nodes {
			if n.XMLName.Local == tagName {
				if count == targetIdx {
					next = n
					break
				}
				count++
			}
		}
		if next == nil {
			for i := count; i <= targetIdx; i++ {
				newNode := &Node{XMLName: xml.Name{Local: tagName}}
				current.Nodes = append(current.Nodes, newNode)
				if i == targetIdx {
					next = newNode
				}
			}
		}
		current = next
	}
	return nil
}

func (x *XmlEngine) Set(path string, val string) error {
	if err := x.ensurePath(path); err != nil {
		return err
	}

	node, _, err := x.find(path)
	if err != nil {
		return err
	}
	node.Content = val
	return nil
}

func (x *XmlEngine) Delete(path string) error {
	node, parent, err := x.find(path)
	if err != nil || parent == nil {
		return errors.New("not found")
	}

	for i, n := range parent.Nodes {
		if n == node {
			parent.Nodes = append(parent.Nodes[:i], parent.Nodes[i+1:]...)
			return nil
		}
	}
	return errors.New("delete failed")
}

func (x *XmlEngine) Save() error {
	if x.Root == nil {
		return errors.New("nothing to save")
	}

	file, err := os.Create(x.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := xml.NewEncoder(file)
	enc.Indent("", "  ")
	return enc.Encode(x.Root)
}

func (n *Node) GetAttr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (n *Node) SetAttr(name, value string) {
	for i, a := range n.Attrs {
		if a.Name.Local == name {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func nodeToJSONValue(n *Node) jsonValue {

	if len(n.Nodes) == 0 && len(n.Attrs) == 0 {
		return jsonValue{Type: "str", Str: n.Content}
	}

	m := make(map[string]jsonValue)

	for _, a := range n.Attrs {
		m["@"+a.Name.Local] = jsonValue{Type: "str", Str: a.Value}
	}

	if strings.TrimSpace(n.Content) != "" {
		m["_text"] = jsonValue{Type: "str", Str: n.Content}
	}

	childGroups := make(map[string][]jsonValue)
	var order []string

	for _, child := range n.Nodes {
		name := child.XMLName.Local
		if _, exists := childGroups[name]; !exists {
			order = append(order, name)
		}
		childGroups[name] = append(childGroups[name], nodeToJSONValue(child))
	}

	for _, name := range order {
		vals := childGroups[name]
		if len(vals) == 1 {
			m[name] = vals[0]
		} else {
			m[name] = jsonValue{Type: "arr", Arr: vals}
		}
	}

	return jsonValue{Type: "map", Map: m}
}

// ------------------------------------------------------------
// vbx_describe
// ------------------------------------------------------------

//go:wasmexport vbx_describe
func vbxDescribe() uint64 {

	entries := []funcDesc{
		{
			Namespace:   "xml",
			Name:        "Load",
			Params:      "path",
			Description: "Lädt eine XML-Datei vom angegebenen Pfad in den Speicher.",
		},
		{
			Namespace:   "xml",
			Name:        "Save",
			Params:      "[path]",
			Description: "Speichert die aktuelle XML-Struktur. Wird ein Pfad angegeben, wird dieser als neues Ziel gesetzt.",
		},
		{
			Namespace:   "xml",
			Name:        "Parse",
			Params:      "xmlContent",
			Description: "Prüft die XML-Struktur. Gibt True zurück wenn valide, oder eine Fehlermeldung bei Syntaxfehlern.",
		},
		{
			Namespace:   "xml",
			Name:        "Get",
			Params:      "xpath",
			Description: "Liest Content oder Attribute (@attr).",
		},
		{
			Namespace:   "xml",
			Name:        "Set",
			Params:      "xpath, value",
			Description: "Setzt Content oder Attribute (@attr).",
		},
		{
			Namespace:   "xml",
			Name:        "Delete",
			Params:      "xpath",
			Description: "Löscht den spezifizierten Knoten und alle seine Unterknoten.",
		},
		{
			Namespace:   "xml",
			Name:        "ToMap",
			Params:      "[xpath]",
			Description: "Konvertiert den geladenen XML-Baum (oder einen Teilbaum ab xpath) in eine Map-Struktur.",
		},
		{
			Namespace:   "xml",
			Name:        "Count",
			Params:      "xpath",
			Description: "Zählt, wie viele Geschwister-Knoten mit demselben Namen unter dem Parent-Pfad existieren.",
		},
		{
			Namespace:   "xml",
			Name:        "Keys",
			Params:      "[xpath]",
			Description: "Gibt ein Array mit den Namen aller direkten Unterknoten des angegebenen Pfads zurück.",
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

	case "Load":
		return packBytes(handleLoad(args))

	case "Save":
		return packBytes(handleSave(args))

	case "Parse":
		return packBytes(handleParse(args))

	case "Get":
		return packBytes(handleGet(args))

	case "Set":
		return packBytes(handleSet(args))

	case "Delete":
		return packBytes(handleDelete(args))

	case "ToMap":
		return packBytes(handleToMap(args))

	case "Count":
		return packBytes(handleCount(args))

	case "Keys":
		return packBytes(handleKeys(args))

	default:
		return packBytes(
			errorResult(
				"unbekannte Funktion: " + name,
			),
		)
	}
}

// ------------------------------------------------------------
// Load / Save / Parse
// ------------------------------------------------------------

func handleLoad(args []jsonValue) []byte {

	path, err := getStringArg(args, 0, "xml.Load", true)

	if err != nil {
		return errorResult(err.Error())
	}

	if err := xmlEngine.Load(path); err != nil {
		return errorResult(err.Error())
	}

	return strResult("OK")
}

func handleSave(args []jsonValue) []byte {

	path, err := getStringArg(args, 0, "xml.Save", false)

	if err != nil {
		return errorResult(err.Error())
	}

	if path != "" {
		xmlEngine.Path = path
	}

	if err := xmlEngine.Save(); err != nil {
		return errorResult(err.Error())
	}

	return strResult("OK")
}

func handleParse(args []jsonValue) []byte {

	content, err := getStringArg(args, 0, "xml.Parse", true)

	if err != nil {
		return errorResult(err.Error())
	}

	dec := xml.NewDecoder(strings.NewReader(content))

	for {
		_, err := dec.Token()

		if err == io.EOF {
			break
		}

		if err != nil {
			return errorResult(err.Error())
		}
	}

	return boolResult(true)
}

// ------------------------------------------------------------
// Get / Set / Delete
// ------------------------------------------------------------

func handleGet(args []jsonValue) []byte {

	path, err := getStringArg(args, 0, "xml.Get", false)

	if err != nil {
		return errorResult(err.Error())
	}

	attrName := ""

	if idx := strings.Index(path, "@"); idx != -1 {
		attrName = path[idx+1:]
		path = strings.TrimSuffix(path[:idx], ".")
	}

	node, _, err := xmlEngine.find(path)

	if err != nil || node == nil {
		return strResult("")
	}

	if attrName != "" {
		return strResult(node.GetAttr(attrName))
	}

	return strResult(node.Content)
}

func handleSet(args []jsonValue) []byte {

	if len(args) < 2 {
		return errorResult("xml.Set: Pfad/Wert fehlt")
	}

	path, err := getStringArg(args, 0, "xml.Set", true)

	if err != nil {
		return errorResult(err.Error())
	}

	val, err := getStringArg(args, 1, "xml.Set", true)

	if err != nil {
		return errorResult(err.Error())
	}

	if idx := strings.Index(path, "@"); idx != -1 {
		attrName := path[idx+1:]
		nodePath := strings.TrimSuffix(path[:idx], ".")

		node, _, _ := xmlEngine.find(nodePath)

		if node == nil {
			if err := xmlEngine.ensurePath(nodePath); err != nil {
				return errorResult(
					"Knoten konnte nicht erstellt werden: " + err.Error(),
				)
			}
			node, _, _ = xmlEngine.find(nodePath)
		}

		if node == nil {
			return errorResult("Knoten konnte nicht erstellt werden")
		}

		node.SetAttr(attrName, val)
		return strResult("OK")
	}

	if err := xmlEngine.Set(path, val); err != nil {
		return errorResult(err.Error())
	}

	return strResult("OK")
}

func handleDelete(args []jsonValue) []byte {

	path, err := getStringArg(args, 0, "xml.Delete", true)

	if err != nil {
		return errorResult(err.Error())
	}

	if err := xmlEngine.Delete(path); err != nil {
		return errorResult(err.Error())
	}

	return strResult("OK")
}

// ------------------------------------------------------------
// ToMap / Count / Keys
// ------------------------------------------------------------

func handleToMap(args []jsonValue) []byte {

	path, err := getStringArg(args, 0, "xml.ToMap", false)

	if err != nil {
		return errorResult(err.Error())
	}

	var target *Node

	if path == "" {
		target = xmlEngine.Root
	} else {
		node, _, err := xmlEngine.find(path)
		if err != nil {
			return errorResult("xml.ToMap: " + err.Error())
		}
		target = node
	}

	if target == nil {
		return errorResult("xml.ToMap: kein XML geladen oder Pfad nicht gefunden")
	}

	return valueResult(nodeToJSONValue(target))
}

func handleCount(args []jsonValue) []byte {

	path, err := getStringArg(args, 0, "xml.Count", true)

	if err != nil {
		return errorResult(err.Error())
	}

	parts := splitPath(path)

	if len(parts) == 0 {
		return numResult(0)
	}

	if len(parts) == 1 {
		if xmlEngine.Root != nil && xmlEngine.Root.XMLName.Local == parts[0] {
			return numResult(1)
		}
		return numResult(0)
	}

	parentPath := strings.Join(parts[:len(parts)-1], ".")
	tagName, _ := parseStep(parts[len(parts)-1])

	parent, _, err := xmlEngine.find(parentPath)

	if err != nil {
		return numResult(0)
	}

	count := 0

	for _, n := range parent.Nodes {
		if n.XMLName.Local == tagName {
			count++
		}
	}

	return numResult(float64(count))
}

func handleKeys(args []jsonValue) []byte {

	path, err := getStringArg(args, 0, "xml.Keys", false)

	if err != nil {
		return errorResult(err.Error())
	}

	var target *Node

	if path == "" {
		target = xmlEngine.Root
	} else {
		node, _, _ := xmlEngine.find(path)
		target = node
	}

	if target == nil {
		return arrayResult(nil)
	}

	names := make([]jsonValue, len(target.Nodes))

	for i, n := range target.Nodes {
		names[i] = jsonValue{Type: "str", Str: n.XMLName.Local}
	}

	return arrayResult(names)
}

// ------------------------------------------------------------

func main() {}
