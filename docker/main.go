package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unsafe"
)

// ============================================================
// WASM Speicher
// ============================================================

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

// ============================================================
// ABI
// ============================================================

//export vbx_abi_version
func vbx_abi_version() int32 {
	return 1
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
// Host Docker Bridge
// ============================================================

type hostDockerRequest struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin,omitempty"`
}

type hostDockerResponse struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Code   int    `json:"code"`
}

//go:wasmimport vbx_host docker_exec
func hostDockerExec(ptr uint32, length uint32) uint64

// ============================================================
// Host File Bridge
// ============================================================

type hostWriteFileRequest struct {
	Path string `json:"path"`
	Data string `json:"data"`
}

type hostWriteFileResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

//go:wasmimport vbx_host write_file
func hostWriteFile(ptr uint32, length uint32) uint64

// ============================================================
// Speicher / Pointer
// ============================================================

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

func packBytes(data []byte) uint64 {
	ptr := alloc(uint32(len(data)))

	if len(data) > 0 {
		copy(liveBuffers[ptr], data)
	}

	return (uint64(ptr) << 32) | uint64(len(data))
}

func unpackPtrLen(packed uint64) (uint32, uint32) {
	return uint32(packed >> 32), uint32(packed)
}

// ============================================================
// Ergebnisse
// ============================================================

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

// ============================================================
// JSON-Werte
// ============================================================

func valueString(v jsonValue) string {
	switch v.Type {
	case "str":
		return v.Str

	case "num":
		return strconv.FormatFloat(
			v.Num,
			'f',
			-1,
			64,
		)

	case "bool":
		if v.Bool {
			return "true"
		}

		return "false"

	default:
		return ""
	}
}

func valueBool(v jsonValue) bool {
	switch v.Type {
	case "bool":
		return v.Bool

	case "num":
		return v.Num != 0

	case "str":
		s := strings.ToLower(
			strings.TrimSpace(v.Str),
		)

		return s == "true" ||
			s == "1" ||
			s == "yes"

	default:
		return false
	}
}

func valueInt(v jsonValue, defaultValue int) int {
	if v.Type != "num" {
		return defaultValue
	}

	return int(v.Num)
}

func requireString(
	args []jsonValue,
	index int,
	name string,
) (string, []byte) {
	if len(args) <= index {
		return "", errorResult(
			fmt.Sprintf(
				"%s erwartet mindestens %d Argument(e)",
				name,
				index+1,
			),
		)
	}

	if args[index].Type != "str" {
		return "", errorResult(
			fmt.Sprintf(
				"%s: Argument %d muss ein String sein",
				name,
				index+1,
			),
		)
	}

	return args[index].Str, nil
}

// ============================================================
// Array als String-Liste
// ============================================================

func stringArray(v jsonValue) ([]string, error) {
	if v.Type != "arr" {
		return nil, fmt.Errorf(
			"erwartet wird ein Array",
		)
	}

	result := make(
		[]string,
		0,
		len(v.Arr),
	)

	for _, item := range v.Arr {
		if item.Type != "str" {
			return nil, fmt.Errorf(
				"Array muss ausschließlich Strings enthalten",
			)
		}

		result = append(
			result,
			item.Str,
		)
	}

	return result, nil
}

// ============================================================
// Docker Host-Aufruf
// ============================================================

func dockerExec(
	args []string,
	stdin string,
) (hostDockerResponse, error) {
	req := hostDockerRequest{
		Args:  args,
		Stdin: stdin,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return hostDockerResponse{}, fmt.Errorf(
			"Request konnte nicht kodiert werden: %v",
			err,
		)
	}

	ptr := alloc(uint32(len(data)))

	copy(
		liveBuffers[ptr],
		data,
	)

	packed := hostDockerExec(
		ptr,
		uint32(len(data)),
	)

	dealloc(
		ptr,
		uint32(len(data)),
	)

	resPtr, resLen := unpackPtrLen(packed)

	if resPtr == 0 || resLen == 0 {
		return hostDockerResponse{}, fmt.Errorf(
			"keine Antwort von der Docker-Host-Bridge",
		)
	}

	resultData := readBytes(
		resPtr,
		resLen,
	)

	if resultData == nil {
		return hostDockerResponse{}, fmt.Errorf(
			"Antwort der Docker-Host-Bridge konnte nicht gelesen werden",
		)
	}

	var result hostDockerResponse

	if err := json.Unmarshal(
		resultData,
		&result,
	); err != nil {
		dealloc(resPtr, resLen)

		return hostDockerResponse{}, fmt.Errorf(
			"ungültige Antwort der Docker-Host-Bridge: %v",
			err,
		)
	}

	dealloc(
		resPtr,
		resLen,
	)

	return result, nil
}

func dockerResult(
	result hostDockerResponse,
) []byte {
	if result.Code != 0 {
		msg := strings.TrimSpace(
			result.Stderr,
		)

		if msg == "" {
			msg = strings.TrimSpace(
				result.Stdout,
			)
		}

		if msg == "" {
			msg = fmt.Sprintf(
				"Docker-Befehl fehlgeschlagen (Exit-Code %d)",
				result.Code,
			)
		}

		return errorResult(msg)
	}

	return strResult(
		result.Stdout,
	)
}

func dockerOK(
	result hostDockerResponse,
) []byte {
	if result.Code != 0 {
		msg := strings.TrimSpace(
			result.Stderr,
		)

		if msg == "" {
			msg = fmt.Sprintf(
				"Docker-Befehl fehlgeschlagen (Exit-Code %d)",
				result.Code,
			)
		}

		return errorResult(msg)
	}

	return boolResult(true)
}

// ============================================================
// Host-Datei schreiben
// ============================================================

func writeHostFile(
	path string,
	data string,
) error {
	req := hostWriteFileRequest{
		Path: path,
		Data: data,
	}

	requestBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf(
			"Request konnte nicht kodiert werden: %v",
			err,
		)
	}

	ptr := alloc(
		uint32(len(requestBytes)),
	)

	copy(
		liveBuffers[ptr],
		requestBytes,
	)

	packed := hostWriteFile(
		ptr,
		uint32(len(requestBytes)),
	)

	dealloc(
		ptr,
		uint32(len(requestBytes)),
	)

	resPtr, resLen := unpackPtrLen(
		packed,
	)

	if resPtr == 0 || resLen == 0 {
		return fmt.Errorf(
			"keine Antwort von write_file",
		)
	}

	responseBytes := readBytes(
		resPtr,
		resLen,
	)

	if responseBytes == nil {
		dealloc(
			resPtr,
			resLen,
		)

		return fmt.Errorf(
			"Antwort von write_file konnte nicht gelesen werden",
		)
	}

	var response hostWriteFileResponse

	if err := json.Unmarshal(
		responseBytes,
		&response,
	); err != nil {
		dealloc(
			resPtr,
			resLen,
		)

		return fmt.Errorf(
			"ungültige write_file-Antwort: %v",
			err,
		)
	}

	dealloc(
		resPtr,
		resLen,
	)

	if !response.OK {
		if response.Error == "" {
			return fmt.Errorf(
				"Datei konnte nicht geschrieben werden",
			)
		}

		return fmt.Errorf(
			"%s",
			response.Error,
		)
	}

	return nil
}

// ============================================================
// CONTAINER
// ============================================================

// docker.List([all])
func handleList(args []jsonValue) []byte {
	all := false

	if len(args) > 0 {
		all = valueBool(args[0])
	}

	dockerArgs := []string{
		"ps",
		"--format",
		"{{.ID}}|{{.Names}}|{{.Image}}|{{.Status}}|{{.Ports}}",
	}

	if all {
		dockerArgs = append(
			dockerArgs,
			"-a",
		)
	}

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	if result.Code != 0 {
		return dockerResult(result)
	}

	lines := strings.Split(
		strings.TrimSpace(result.Stdout),
		"\n",
	)

	if len(lines) == 1 &&
		lines[0] == "" {
		lines = []string{}
	}

	arr := make(
		[]jsonValue,
		0,
		len(lines),
	)

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			arr = append(
				arr,
				jsonValue{
					Type: "str",
					Str:  line,
				},
			)
		}
	}

	return valueResult(
		jsonValue{
			Type: "arr",
			Arr:  arr,
		},
	)
}

// docker.Run(image, [name], [options], [command])
func handleRun(args []jsonValue) []byte {
	image, errResult := requireString(
		args,
		0,
		"docker.Run",
	)

	if errResult != nil {
		return errResult
	}

	dockerArgs := []string{
		"run",
		"-d",
	}

	argIndex := 1

	// --------------------------------------------------------
	// Name
	// --------------------------------------------------------

	if len(args) > argIndex &&
		args[argIndex].Type == "str" {

		if args[argIndex].Str != "" {
			dockerArgs = append(
				dockerArgs,
				"--name",
				args[argIndex].Str,
			)
		}

		argIndex++
	}

	// --------------------------------------------------------
	// Options
	// --------------------------------------------------------

	if len(args) > argIndex &&
		args[argIndex].Type == "arr" {

		options, err := stringArray(
			args[argIndex],
		)

		if err != nil {
			return errorResult(
				"docker.Run: " + err.Error(),
			)
		}

		dockerArgs = append(
			dockerArgs,
			options...,
		)

		argIndex++
	}

	// --------------------------------------------------------
	// Image
	// --------------------------------------------------------

	dockerArgs = append(
		dockerArgs,
		image,
	)

	// --------------------------------------------------------
	// Command
	// --------------------------------------------------------

	if len(args) > argIndex &&
		args[argIndex].Type == "arr" {

		command, err := stringArray(
			args[argIndex],
		)

		if err != nil {
			return errorResult(
				"docker.Run: " + err.Error(),
			)
		}

		dockerArgs = append(
			dockerArgs,
			command...,
		)
	}

	// --------------------------------------------------------
	// Image automatisch ziehen
	// --------------------------------------------------------

	inspect, _ := dockerExec(
		[]string{
			"image",
			"inspect",
			image,
		},
		"",
	)

	if inspect.Code != 0 {
		pull, err := dockerExec(
			[]string{
				"pull",
				image,
			},
			"",
		)

		if err != nil {
			return errorResult(err.Error())
		}

		if pull.Code != 0 {
			return dockerResult(pull)
		}
	}

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.Login(registry, username, token)
func handleLogin(args []jsonValue) []byte {
	registry, errResult := requireString(
		args,
		0,
		"docker.Login",
	)

	if errResult != nil {
		return errResult
	}

	username, errResult := requireString(
		args,
		1,
		"docker.Login",
	)

	if errResult != nil {
		return errResult
	}

	token, errResult := requireString(
		args,
		2,
		"docker.Login",
	)

	if errResult != nil {
		return errResult
	}

	result, err := dockerExec(
		[]string{
			"login",
			registry,
			"-u",
			username,
			"--password-stdin",
		},
		token,
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerOK(result)
}

// docker.Logout(registry)
func handleLogout(args []jsonValue) []byte {
	registry, errResult := requireString(
		args,
		0,
		"docker.Logout",
	)

	if errResult != nil {
		return errResult
	}

	result, err := dockerExec(
		[]string{
			"logout",
			registry,
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerOK(result)
}

// docker.Start(name)
func handleStart(args []jsonValue) []byte {
	name, errResult := requireString(
		args,
		0,
		"docker.Start",
	)

	if errResult != nil {
		return errResult
	}

	result, err := dockerExec(
		[]string{
			"start",
			name,
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.Stop(name, [timeout])
func handleStop(args []jsonValue) []byte {
	name, errResult := requireString(
		args,
		0,
		"docker.Stop",
	)

	if errResult != nil {
		return errResult
	}

	timeout := 10

	if len(args) > 1 {
		timeout = valueInt(
			args[1],
			10,
		)
	}

	result, err := dockerExec(
		[]string{
			"stop",
			"-t",
			strconv.Itoa(timeout),
			name,
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.Restart(name, [timeout])
func handleRestart(args []jsonValue) []byte {
	name, errResult := requireString(
		args,
		0,
		"docker.Restart",
	)

	if errResult != nil {
		return errResult
	}

	timeout := 10

	if len(args) > 1 {
		timeout = valueInt(
			args[1],
			10,
		)
	}

	result, err := dockerExec(
		[]string{
			"restart",
			"-t",
			strconv.Itoa(timeout),
			name,
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.Remove(name, [force])
func handleRemove(args []jsonValue) []byte {
	name, errResult := requireString(
		args,
		0,
		"docker.Remove",
	)

	if errResult != nil {
		return errResult
	}

	dockerArgs := []string{
		"rm",
	}

	if len(args) > 1 &&
		valueBool(args[1]) {

		dockerArgs = append(
			dockerArgs,
			"-f",
		)
	}

	dockerArgs = append(
		dockerArgs,
		name,
	)

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.Status(name)
func handleStatus(args []jsonValue) []byte {
	name, errResult := requireString(
		args,
		0,
		"docker.Status",
	)

	if errResult != nil {
		return errResult
	}

	result, err := dockerExec(
		[]string{
			"inspect",
			"--format",
			"{{.State.Status}}",
			name,
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.IsRunning(name)
func handleIsRunning(args []jsonValue) []byte {
	name, errResult := requireString(
		args,
		0,
		"docker.IsRunning",
	)

	if errResult != nil {
		return errResult
	}

	result, err := dockerExec(
		[]string{
			"inspect",
			"--format",
			"{{.State.Running}}",
			name,
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	if result.Code != 0 {
		return boolResult(false)
	}

	return boolResult(
		strings.EqualFold(
			strings.TrimSpace(result.Stdout),
			"true",
		),
	)
}

// docker.Inspect(name)
func handleInspect(args []jsonValue) []byte {
	name, errResult := requireString(
		args,
		0,
		"docker.Inspect",
	)

	if errResult != nil {
		return errResult
	}

	result, err := dockerExec(
		[]string{
			"inspect",
			name,
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.Logs(name, [lines])
func handleLogs(args []jsonValue) []byte {
	name, errResult := requireString(
		args,
		0,
		"docker.Logs",
	)

	if errResult != nil {
		return errResult
	}

	lines := 50

	if len(args) > 1 {
		lines = valueInt(
			args[1],
			50,
		)
	}

	result, err := dockerExec(
		[]string{
			"logs",
			"--tail",
			strconv.Itoa(lines),
			name,
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.Exec(name, cmd, [args...])
func handleExec(args []jsonValue) []byte {
	name, errResult := requireString(
		args,
		0,
		"docker.Exec",
	)

	if errResult != nil {
		return errResult
	}

	command, errResult := requireString(
		args,
		1,
		"docker.Exec",
	)

	if errResult != nil {
		return errResult
	}

	dockerArgs := []string{
		"exec",
		name,
		command,
	}

	if len(args) > 2 {
		for i := 2; i < len(args); i++ {
			if args[i].Type != "str" {
				return errorResult(
					"docker.Exec: Argumente ab Position 3 müssen Strings sein",
				)
			}

			dockerArgs = append(
				dockerArgs,
				args[i].Str,
			)
		}
	}

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.Stats([name])
func handleStats(args []jsonValue) []byte {
	dockerArgs := []string{
		"stats",
		"--no-stream",
		"--format",
		"{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}|{{.MemPerc}}",
	}

	if len(args) > 0 &&
		args[0].Type == "str" &&
		args[0].Str != "" {

		dockerArgs = append(
			dockerArgs,
			args[0].Str,
		)
	}

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	if result.Code != 0 {
		return dockerResult(result)
	}

	lines := strings.Split(
		strings.TrimSpace(result.Stdout),
		"\n",
	)

	arr := make(
		[]jsonValue,
		0,
		len(lines),
	)

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		arr = append(
			arr,
			jsonValue{
				Type: "str",
				Str:  line,
			},
		)
	}

	return valueResult(
		jsonValue{
			Type: "arr",
			Arr:  arr,
		},
	)
}

// ============================================================
// EXPORT COMPOSE
// ============================================================

// YAML-String sicher in einfache Quotes setzen.
//
// Beispiel:
//
// abc        -> 'abc'
// O'Reilly  -> 'O”Reilly'
func yamlQuote(value string) string {
	value = strings.ReplaceAll(
		value,
		"'",
		"''",
	)

	return "'" + value + "'"
}

// ============================================================
// Docker Inspect Hilfsfunktionen
// ============================================================

func inspectContainer(
	name string,
) (map[string]interface{}, error) {

	result, err := dockerExec(
		[]string{
			"inspect",
			name,
		},
		"",
	)

	if err != nil {
		return nil, err
	}

	if result.Code != 0 {
		msg := strings.TrimSpace(
			result.Stderr,
		)

		if msg == "" {
			msg = strings.TrimSpace(
				result.Stdout,
			)
		}

		if msg == "" {
			msg = fmt.Sprintf(
				"docker inspect fehlgeschlagen (Exit-Code %d)",
				result.Code,
			)
		}

		return nil, fmt.Errorf(
			"docker inspect: %s",
			msg,
		)
	}

	var containers []map[string]interface{}

	if err := json.Unmarshal(
		[]byte(result.Stdout),
		&containers,
	); err != nil {
		return nil, fmt.Errorf(
			"Inspect-JSON konnte nicht gelesen werden: %v",
			err,
		)
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf(
			"Container nicht gefunden: %s",
			name,
		)
	}

	return containers[0], nil
}

func mapValue(
	m map[string]interface{},
	key string,
) map[string]interface{} {

	if m == nil {
		return nil
	}

	value, ok := m[key]

	if !ok {
		return nil
	}

	result, ok := value.(map[string]interface{})

	if !ok {
		return nil
	}

	return result
}

func mapString(
	m map[string]interface{},
	key string,
) string {

	if m == nil {
		return ""
	}

	value, ok := m[key]

	if !ok {
		return ""
	}

	result, ok := value.(string)

	if !ok {
		return ""
	}

	return result
}

// ============================================================
// Compose YAML erzeugen
// ============================================================

func buildComposeYAML(
	container map[string]interface{},
	fallbackName string,
) string {

	config := mapValue(
		container,
		"Config",
	)

	hostConfig := mapValue(
		container,
		"HostConfig",
	)

	// --------------------------------------------------------
	// Containername
	// --------------------------------------------------------

	name := strings.TrimPrefix(
		mapString(
			container,
			"Name",
		),
		"/",
	)

	if name == "" {
		name = fallbackName
	}

	// --------------------------------------------------------
	// Image
	// --------------------------------------------------------

	image := mapString(
		config,
		"Image",
	)

	var b strings.Builder

	b.WriteString(
		"version: '3'\n",
	)

	b.WriteString(
		"services:\n",
	)

	b.WriteString(
		"  ",
	)

	b.WriteString(
		yamlQuote(name),
	)

	b.WriteString(
		":\n",
	)

	if image != "" {
		b.WriteString(
			"    image: ",
		)

		b.WriteString(
			yamlQuote(image),
		)

		b.WriteString(
			"\n",
		)
	}

	// --------------------------------------------------------
	// Restart Policy
	// --------------------------------------------------------

	if hostConfig != nil {

		restartPolicy := mapValue(
			hostConfig,
			"RestartPolicy",
		)

		if restartPolicy != nil {

			restartName := mapString(
				restartPolicy,
				"Name",
			)

			if restartName != "" &&
				restartName != "no" {

				b.WriteString(
					"    restart: ",
				)

				b.WriteString(
					yamlQuote(restartName),
				)

				b.WriteString(
					"\n",
				)
			}
		}
	}

	// --------------------------------------------------------
	// Environment
	// --------------------------------------------------------

	if config != nil {

		envRaw, ok := config["Env"]

		if ok {

			env, ok := envRaw.([]interface{})

			if ok && len(env) > 0 {

				b.WriteString(
					"    environment:\n",
				)

				for _, item := range env {

					value, ok := item.(string)

					if !ok {
						continue
					}

					b.WriteString(
						"      - ",
					)

					b.WriteString(
						yamlQuote(value),
					)

					b.WriteString(
						"\n",
					)
				}
			}
		}
	}

	// --------------------------------------------------------
	// Labels
	// --------------------------------------------------------

	if config != nil {

		labels := mapValue(
			config,
			"Labels",
		)

		if len(labels) > 0 {

			b.WriteString(
				"    labels:\n",
			)

			for key, value := range labels {

				if value == nil {
					value = ""
				}

				b.WriteString(
					"      ",
				)

				b.WriteString(
					yamlQuote(key),
				)

				b.WriteString(
					": ",
				)

				b.WriteString(
					yamlQuote(
						fmt.Sprint(value),
					),
				)

				b.WriteString(
					"\n",
				)
			}
		}
	}

	// --------------------------------------------------------
	// Ports
	// --------------------------------------------------------

	if hostConfig != nil {

		portBindings := mapValue(
			hostConfig,
			"PortBindings",
		)

		if len(portBindings) > 0 {

			b.WriteString(
				"    ports:\n",
			)

			for containerPort, rawBindings := range portBindings {

				bindings, ok := rawBindings.([]interface{})

				if !ok {
					continue
				}

				for _, rawBinding := range bindings {

					binding, ok :=
						rawBinding.(map[string]interface{})

					if !ok {
						continue
					}

					hostIP := mapString(
						binding,
						"HostIp",
					)

					hostPort := mapString(
						binding,
						"HostPort",
					)

					port := containerPort

					if hostPort != "" {

						if hostIP != "" &&
							hostIP != "0.0.0.0" {

							port =
								hostIP +
									":" +
									hostPort +
									":" +
									containerPort

						} else {

							port =
								hostPort +
									":" +
									containerPort
						}
					}

					b.WriteString(
						"      - ",
					)

					b.WriteString(
						yamlQuote(port),
					)

					b.WriteString(
						"\n",
					)
				}
			}
		}
	}

	// --------------------------------------------------------
	// Volumes / Mounts
	// --------------------------------------------------------

	if mountsRaw, ok := container["Mounts"]; ok {

		if mounts, ok := mountsRaw.([]interface{}); ok &&
			len(mounts) > 0 {

			b.WriteString(
				"    volumes:\n",
			)

			for _, rawMount := range mounts {

				mount, ok :=
					rawMount.(map[string]interface{})

				if !ok {
					continue
				}

				source := mapString(
					mount,
					"Source",
				)

				destination := mapString(
					mount,
					"Destination",
				)

				if source == "" ||
					destination == "" {

					continue
				}

				value :=
					source +
						":" +
						destination

				rw, ok := mount["RW"].(bool)

				if ok && !rw {
					value += ":ro"
				}

				b.WriteString(
					"      - ",
				)

				b.WriteString(
					yamlQuote(value),
				)

				b.WriteString(
					"\n",
				)
			}
		}
	}

	return b.String()
}

// ============================================================
// docker.ExportCompose(name, [outputPath])
// ============================================================

func handleExportCompose(
	args []jsonValue,
) []byte {

	name, errResult := requireString(
		args,
		0,
		"docker.ExportCompose",
	)

	if errResult != nil {
		return errResult
	}

	// --------------------------------------------------------
	// Container inspizieren
	// --------------------------------------------------------

	container, err := inspectContainer(
		name,
	)

	if err != nil {
		return errorResult(
			err.Error(),
		)
	}

	// --------------------------------------------------------
	// Compose YAML erzeugen
	// --------------------------------------------------------

	yaml := buildComposeYAML(
		container,
		name,
	)

	// --------------------------------------------------------
	// Kein outputPath:
	// YAML direkt zurückgeben
	// --------------------------------------------------------

	if len(args) < 2 ||
		args[1].Type != "str" ||
		args[1].Str == "" {

		return strResult(
			yaml,
		)
	}

	// --------------------------------------------------------
	// YAML auf Host schreiben
	// --------------------------------------------------------

	outputPath := args[1].Str

	if err := writeHostFile(
		outputPath,
		yaml,
	); err != nil {

		return errorResult(
			"Compose-Datei konnte nicht geschrieben werden: " +
				err.Error(),
		)
	}

	return strResult("OK")
}

// ============================================================
// IMAGES
// ============================================================

// docker.ImageList()
func handleImageList(args []jsonValue) []byte {
	result, err := dockerExec(
		[]string{
			"image",
			"ls",
			"--format",
			"{{.Repository}}|{{.Tag}}|{{.ID}}|{{.CreatedSince}}|{{.Size}}",
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	if result.Code != 0 {
		return dockerResult(result)
	}

	lines := strings.Split(
		strings.TrimSpace(result.Stdout),
		"\n",
	)

	arr := make(
		[]jsonValue,
		0,
		len(lines),
	)

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		arr = append(
			arr,
			jsonValue{
				Type: "str",
				Str:  line,
			},
		)
	}

	return valueResult(
		jsonValue{
			Type: "arr",
			Arr:  arr,
		},
	)
}

// docker.ImagePull(image)
func handleImagePull(args []jsonValue) []byte {
	image, errResult := requireString(
		args,
		0,
		"docker.ImagePull",
	)

	if errResult != nil {
		return errResult
	}

	result, err := dockerExec(
		[]string{
			"pull",
			image,
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.ImageRemove(image, [force])
func handleImageRemove(args []jsonValue) []byte {
	image, errResult := requireString(
		args,
		0,
		"docker.ImageRemove",
	)

	if errResult != nil {
		return errResult
	}

	dockerArgs := []string{
		"rmi",
	}

	if len(args) > 1 &&
		valueBool(args[1]) {

		dockerArgs = append(
			dockerArgs,
			"-f",
		)
	}

	dockerArgs = append(
		dockerArgs,
		image,
	)

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.ImageBuild(path, tag, [dockerfile])
func handleImageBuild(args []jsonValue) []byte {
	path, errResult := requireString(
		args,
		0,
		"docker.ImageBuild",
	)

	if errResult != nil {
		return errResult
	}

	tag, errResult := requireString(
		args,
		1,
		"docker.ImageBuild",
	)

	if errResult != nil {
		return errResult
	}

	dockerArgs := []string{
		"build",
		"-t",
		tag,
	}

	if len(args) > 2 {

		dockerfile, errResult := requireString(
			args,
			2,
			"docker.ImageBuild",
		)

		if errResult != nil {
			return errResult
		}

		dockerArgs = append(
			dockerArgs,
			"-f",
			dockerfile,
		)
	}

	dockerArgs = append(
		dockerArgs,
		path,
	)

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.ImagePrune()
func handleImagePrune(args []jsonValue) []byte {
	result, err := dockerExec(
		[]string{
			"image",
			"prune",
			"-f",
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.ImageRemoveAll()
func handleImageRemoveAll(args []jsonValue) []byte {
	images, err := dockerExec(
		[]string{
			"image",
			"ls",
			"-aq",
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	if images.Code != 0 {
		return dockerResult(images)
	}

	lines := strings.Fields(
		images.Stdout,
	)

	if len(lines) == 0 {
		return strResult("OK")
	}

	dockerArgs := []string{
		"rmi",
		"-f",
	}

	dockerArgs = append(
		dockerArgs,
		lines...,
	)

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// ============================================================
// COMPOSE
// ============================================================

func composeCommand(
	command string,
	path string,
) []string {
	return []string{
		"compose",
		"-f",
		path,
		command,
	}
}

// docker.ComposeUp(path, [detach])
func handleComposeUp(args []jsonValue) []byte {
	path, errResult := requireString(
		args,
		0,
		"docker.ComposeUp",
	)

	if errResult != nil {
		return errResult
	}

	dockerArgs := composeCommand(
		"up",
		path,
	)

	if len(args) > 1 &&
		valueBool(args[1]) {

		dockerArgs = append(
			dockerArgs,
			"-d",
		)
	}

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.ComposeDown(path, [removeVolumes])
func handleComposeDown(args []jsonValue) []byte {
	path, errResult := requireString(
		args,
		0,
		"docker.ComposeDown",
	)

	if errResult != nil {
		return errResult
	}

	dockerArgs := composeCommand(
		"down",
		path,
	)

	if len(args) > 1 &&
		valueBool(args[1]) {

		dockerArgs = append(
			dockerArgs,
			"-v",
		)
	}

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.ComposePull(path)
func handleComposePull(args []jsonValue) []byte {
	path, errResult := requireString(
		args,
		0,
		"docker.ComposePull",
	)

	if errResult != nil {
		return errResult
	}

	result, err := dockerExec(
		composeCommand(
			"pull",
			path,
		),
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.ComposeRestart(path)
func handleComposeRestart(args []jsonValue) []byte {
	path, errResult := requireString(
		args,
		0,
		"docker.ComposeRestart",
	)

	if errResult != nil {
		return errResult
	}

	result, err := dockerExec(
		composeCommand(
			"restart",
			path,
		),
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.ComposeLogs(path, [lines])
func handleComposeLogs(args []jsonValue) []byte {
	path, errResult := requireString(
		args,
		0,
		"docker.ComposeLogs",
	)

	if errResult != nil {
		return errResult
	}

	lines := 50

	if len(args) > 1 {
		lines = valueInt(
			args[1],
			50,
		)
	}

	dockerArgs := composeCommand(
		"logs",
		path,
	)

	dockerArgs = append(
		dockerArgs,
		"--tail",
		strconv.Itoa(lines),
	)

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// ============================================================
// SYSTEM
// ============================================================

// docker.Version()
func handleVersion(args []jsonValue) []byte {
	if len(args) != 0 {
		return errorResult(
			"docker.Version() erwartet keine Argumente",
		)
	}

	result, err := dockerExec(
		[]string{
			"version",
			"--format",
			"{{.Server.Version}}",
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.IsInstalled()
func handleIsInstalled(args []jsonValue) []byte {
	if len(args) != 0 {
		return errorResult(
			"docker.IsInstalled() erwartet keine Argumente",
		)
	}

	result, err := dockerExec(
		[]string{
			"version",
			"--format",
			"{{.Server.Version}}",
		},
		"",
	)

	if err != nil {
		return boolResult(false)
	}

	return boolResult(
		result.Code == 0,
	)
}

// docker.SystemPrune([all])
func handleSystemPrune(args []jsonValue) []byte {
	dockerArgs := []string{
		"system",
		"prune",
		"-f",
	}

	if len(args) > 0 &&
		valueBool(args[0]) {

		dockerArgs = append(
			dockerArgs,
			"-a",
		)
	}

	result, err := dockerExec(
		dockerArgs,
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	return dockerResult(result)
}

// docker.NetworkList()
func handleNetworkList(args []jsonValue) []byte {
	result, err := dockerExec(
		[]string{
			"network",
			"ls",
			"--format",
			"{{.ID}}|{{.Name}}|{{.Driver}}|{{.Scope}}",
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	if result.Code != 0 {
		return dockerResult(result)
	}

	lines := strings.Split(
		strings.TrimSpace(result.Stdout),
		"\n",
	)

	arr := make(
		[]jsonValue,
		0,
		len(lines),
	)

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		arr = append(
			arr,
			jsonValue{
				Type: "str",
				Str:  line,
			},
		)
	}

	return valueResult(
		jsonValue{
			Type: "arr",
			Arr:  arr,
		},
	)
}

// docker.VolumeList()
func handleVolumeList(args []jsonValue) []byte {
	result, err := dockerExec(
		[]string{
			"volume",
			"ls",
			"--format",
			"{{.Name}}|{{.Driver}}|{{.Scope}}",
		},
		"",
	)

	if err != nil {
		return errorResult(err.Error())
	}

	if result.Code != 0 {
		return dockerResult(result)
	}

	lines := strings.Split(
		strings.TrimSpace(result.Stdout),
		"\n",
	)

	arr := make(
		[]jsonValue,
		0,
		len(lines),
	)

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		arr = append(
			arr,
			jsonValue{
				Type: "str",
				Str:  line,
			},
		)
	}

	return valueResult(
		jsonValue{
			Type: "arr",
			Arr:  arr,
		},
	)
}

// ============================================================
// vbx_describe
// ============================================================

//export vbx_describe
func vbx_describe() uint64 {
	desc := []funcDesc{

		// --------------------------------------------------------
		// CONTAINER
		// --------------------------------------------------------

		{
			Namespace:   "docker",
			Name:        "List",
			Params:      "[all]",
			Description: "Listet Docker-Container auf. Mit all=True werden auch gestoppte Container angezeigt.",
		},

		{
			Namespace:   "docker",
			Name:        "Run",
			Params:      "image, [name], [options], [command]",
			Description: "Startet einen Docker-Container im Hintergrund.",
		},

		{
			Namespace:   "docker",
			Name:        "Login",
			Params:      "registry, username, token",
			Description: "Meldet sich mit einem Benutzer und Token an einer Docker Registry an.",
		},

		{
			Namespace:   "docker",
			Name:        "Logout",
			Params:      "registry",
			Description: "Meldet sich von einer Docker Registry ab.",
		},

		{
			Namespace:   "docker",
			Name:        "Start",
			Params:      "name",
			Description: "Startet einen gestoppten Docker-Container.",
		},

		{
			Namespace:   "docker",
			Name:        "Stop",
			Params:      "name, [timeout]",
			Description: "Stoppt einen Docker-Container.",
		},

		{
			Namespace:   "docker",
			Name:        "Restart",
			Params:      "name, [timeout]",
			Description: "Startet einen Docker-Container neu.",
		},

		{
			Namespace:   "docker",
			Name:        "Remove",
			Params:      "name, [force]",
			Description: "Entfernt einen Docker-Container.",
		},

		{
			Namespace:   "docker",
			Name:        "Status",
			Params:      "name",
			Description: "Gibt den Status eines Docker-Containers zurück.",
		},

		{
			Namespace:   "docker",
			Name:        "IsRunning",
			Params:      "name",
			Description: "Prüft, ob ein Docker-Container läuft.",
		},

		{
			Namespace:   "docker",
			Name:        "Inspect",
			Params:      "name",
			Description: "Gibt die vollständigen Docker-Inspect-Daten eines Containers zurück.",
		},

		{
			Namespace:   "docker",
			Name:        "ExportCompose",
			Params:      "name, [outputPath]",
			Description: "Erzeugt aus einem Docker-Container eine Docker-Compose-Konfiguration und gibt sie zurück oder schreibt sie in eine Datei.",
		},

		{
			Namespace:   "docker",
			Name:        "Logs",
			Params:      "name, [lines]",
			Description: "Gibt die letzten Logzeilen eines Containers zurück.",
		},

		{
			Namespace:   "docker",
			Name:        "Exec",
			Params:      "name, cmd, [args...]",
			Description: "Führt einen Befehl innerhalb eines laufenden Containers aus.",
		},

		{
			Namespace:   "docker",
			Name:        "Stats",
			Params:      "[name]",
			Description: "Gibt die aktuellen Ressourcenstatistiken eines Containers oder aller Container zurück.",
		},

		// --------------------------------------------------------
		// IMAGES
		// --------------------------------------------------------

		{
			Namespace:   "docker",
			Name:        "ImageList",
			Params:      "",
			Description: "Listet vorhandene Docker Images auf.",
		},

		{
			Namespace:   "docker",
			Name:        "ImagePull",
			Params:      "image",
			Description: "Lädt ein Docker Image aus einer Registry.",
		},

		{
			Namespace:   "docker",
			Name:        "ImageRemove",
			Params:      "image, [force]",
			Description: "Entfernt ein Docker Image.",
		},

		{
			Namespace:   "docker",
			Name:        "ImageBuild",
			Params:      "path, tag, [dockerfile]",
			Description: "Baut ein Docker Image aus einem Build-Kontext.",
		},

		{
			Namespace:   "docker",
			Name:        "ImagePrune",
			Params:      "",
			Description: "Entfernt nicht verwendete Docker Images.",
		},

		{
			Namespace:   "docker",
			Name:        "ImageRemoveAll",
			Params:      "",
			Description: "Entfernt alle vorhandenen Docker Images.",
		},

		// --------------------------------------------------------
		// COMPOSE
		// --------------------------------------------------------

		{
			Namespace:   "docker",
			Name:        "ComposeUp",
			Params:      "path, [detach]",
			Description: "Startet einen Docker-Compose-Stack.",
		},

		{
			Namespace:   "docker",
			Name:        "ComposeDown",
			Params:      "path, [removeVolumes]",
			Description: "Stoppt und entfernt einen Docker-Compose-Stack.",
		},

		{
			Namespace:   "docker",
			Name:        "ComposePull",
			Params:      "path",
			Description: "Lädt die Images eines Docker-Compose-Stacks herunter.",
		},

		{
			Namespace:   "docker",
			Name:        "ComposeRestart",
			Params:      "path",
			Description: "Startet einen Docker-Compose-Stack neu.",
		},

		{
			Namespace:   "docker",
			Name:        "ComposeLogs",
			Params:      "path, [lines]",
			Description: "Gibt die Logs eines Docker-Compose-Stacks zurück.",
		},

		// --------------------------------------------------------
		// SYSTEM
		// --------------------------------------------------------

		{
			Namespace:   "docker",
			Name:        "Version",
			Params:      "",
			Description: "Gibt die Version des Docker Servers zurück.",
		},

		{
			Namespace:   "docker",
			Name:        "IsInstalled",
			Params:      "",
			Description: "Prüft, ob Docker installiert und erreichbar ist.",
		},

		{
			Namespace:   "docker",
			Name:        "SystemPrune",
			Params:      "[all]",
			Description: "Entfernt nicht verwendete Docker-Ressourcen.",
		},

		{
			Namespace:   "docker",
			Name:        "NetworkList",
			Params:      "",
			Description: "Listet Docker-Netzwerke auf.",
		},

		{
			Namespace:   "docker",
			Name:        "VolumeList",
			Params:      "",
			Description: "Listet Docker-Volumes auf.",
		},
	}

	data, err := json.Marshal(desc)

	if err != nil {
		return packBytes(
			errorResult(
				"Beschreibung konnte nicht erzeugt werden: " +
					err.Error(),
			),
		)
	}

	return packBytes(data)
}

// ============================================================
// vbx_call
// ============================================================

//export vbx_call
func vbx_call(
	namePtr,
	nameLen,
	argsPtr,
	argsLen uint32,
) uint64 {

	nameBytes := readBytes(
		namePtr,
		nameLen,
	)

	if nameBytes == nil {
		return packBytes(
			errorResult(
				"Funktionsname konnte nicht gelesen werden",
			),
		)
	}

	name := string(nameBytes)

	argsBytes := readBytes(
		argsPtr,
		argsLen,
	)

	if argsBytes == nil {
		return packBytes(
			errorResult(
				"Argumente konnten nicht gelesen werden",
			),
		)
	}

	var args []jsonValue

	if err := json.Unmarshal(
		argsBytes,
		&args,
	); err != nil {

		return packBytes(
			errorResult(
				"Ungültige Argumente: " +
					err.Error(),
			),
		)
	}

	var result []byte

	switch name {

	// --------------------------------------------------------
	// CONTAINER
	// --------------------------------------------------------

	case "List":
		result = handleList(args)

	case "Run":
		result = handleRun(args)

	case "Login":
		result = handleLogin(args)

	case "Logout":
		result = handleLogout(args)

	case "Start":
		result = handleStart(args)

	case "Stop":
		result = handleStop(args)

	case "Restart":
		result = handleRestart(args)

	case "Remove":
		result = handleRemove(args)

	case "Status":
		result = handleStatus(args)

	case "IsRunning":
		result = handleIsRunning(args)

	case "Inspect":
		result = handleInspect(args)

	case "ExportCompose":
		result = handleExportCompose(args)

	case "Logs":
		result = handleLogs(args)

	case "Exec":
		result = handleExec(args)

	case "Stats":
		result = handleStats(args)

	// --------------------------------------------------------
	// IMAGES
	// --------------------------------------------------------

	case "ImageList":
		result = handleImageList(args)

	case "ImagePull":
		result = handleImagePull(args)

	case "ImageRemove":
		result = handleImageRemove(args)

	case "ImageBuild":
		result = handleImageBuild(args)

	case "ImagePrune":
		result = handleImagePrune(args)

	case "ImageRemoveAll":
		result = handleImageRemoveAll(args)

	// --------------------------------------------------------
	// COMPOSE
	// --------------------------------------------------------

	case "ComposeUp":
		result = handleComposeUp(args)

	case "ComposeDown":
		result = handleComposeDown(args)

	case "ComposePull":
		result = handleComposePull(args)

	case "ComposeRestart":
		result = handleComposeRestart(args)

	case "ComposeLogs":
		result = handleComposeLogs(args)

	// --------------------------------------------------------
	// SYSTEM
	// --------------------------------------------------------

	case "Version":
		result = handleVersion(args)

	case "IsInstalled":
		result = handleIsInstalled(args)

	case "SystemPrune":
		result = handleSystemPrune(args)

	case "NetworkList":
		result = handleNetworkList(args)

	case "VolumeList":
		result = handleVolumeList(args)

	default:
		result = errorResult(
			"Unbekannte Docker-Funktion: " +
				name,
		)
	}

	return packBytes(result)
}

// ============================================================
// Beschreibung
// ============================================================

type funcDesc struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Params      string `json:"params"`
	Description string `json:"description"`
}

// ============================================================
// main
// ============================================================

func main() {}
