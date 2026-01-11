package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	cpy3 "go.nhat.io/cpy/v3"
	"go.nhat.io/python/v3"
)

// Configuration
var (
	MQTTBroker = getEnv("MQTT_BROKER", "tcp://localhost:1883")
	ActionsDir = getEnv("ACTIONS_DIR", "../actions")
	AppDir     = getEnv("APP_DIR", "../app")
)

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type Action struct {
	Name         string                 `json:"name"`
	Group        string                 `json:"group"`
	Description  string                 `json:"description"`
	Parameters   map[string]interface{} `json:"parameters"`
	TriggerTopic string                 `json:"trigger_topic"`
	Function     *python.Object         `json:"-"`
}

var (
	actions         = make(map[string]*Action)
	actionsMu       sync.RWMutex
	execMu          sync.Mutex
	stateMu         sync.Mutex
	currentLogTopic string
)

func init() {
	// Initialize Python Interpreter
	// nhatthm/go-python init via cpy3 usually?
	// doc said python.Initialize exists? No, doc summary missed it or I missed it.
	// doc said "package python provides a high-level interface".
	// I will use cpy3.Py_Initialize() just in case if python.Initialize() is missing.
	// But previous error complained python.PyList_Append missing, not python package missing.
	// Let's rely on cpy3.Py_Initialize().
	cpy3.Py_Initialize()
}

func main() {
	setupPython()
	discoverActions()

	opts := mqtt.NewClientOptions().AddBroker(MQTTBroker)
	clientID := "tinpot-worker-" + uuid.New().String()
	opts.SetClientID(clientID)
	opts.SetAutoReconnect(true)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("Connected to MQTT Broker")
		announceActions(c)
		subscribeToActions(c)
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to connect to MQTT: %v", token.Error())
	}

	setupLogCapture(client)
	select {}
}

func setupPython() {
	sys, err := python.ImportModule("sys")
	if err != nil {
		log.Fatal(err)
	}
	path := sys.GetAttr("path")

	cwd, _ := os.Getwd()

	// Add app/lib/etc to path
	// We added AppDir ("../app") before. Now we want "../lib" for the new tinpot package.
	// But "actions" usually imports "tinpot".
	// If we put "lib" in path, then "import tinpot" works (finding "lib/tinpot").
	libDir := getEnv("LIB_DIR", "../lib")

	path.CallMethodArgs("append", libDir)
	// We can keep AppDir if needed, but if we want to avoid conflict, maybe remove it?
	// But actions might depend on other things in app?
	// For now, let's ADD libDir. Using Append puts it at end.
	// If "app" is also in path and has "tinpot", it might shadow it.
	// We should Insert(0, libDir) to prioritize.
	path.CallMethodArgs("insert", 0, libDir)
	path.CallMethodArgs("append", cwd)
	path.CallMethodArgs("append", ActionsDir)
}

func discoverActions() {
	actionsMu.Lock()
	defer actionsMu.Unlock()
	log.Printf("Discovering actions in %s...", ActionsDir)

	loader, err := python.ImportModule("tinpot.loader")
	if err != nil {
		log.Fatalf("Failed to import tinpot.loader: %v", err)
	}

	discoverFunc := loader.GetAttr("discover_actions")
	// Call discover_actions(ActionsDir)
	discoverFunc.CallMethodArgs("__call__", ActionsDir)

	decorators, err := python.ImportModule("tinpot.decorators")
	if err != nil {
		log.Fatal(err)
	}
	registry := decorators.GetAttr("ACTION_REGISTRY")
	// registry is Dict

	// Convert to map
	// Iterate keys
	// keys()
	keysFunc := registry.GetAttr("keys")
	keysObj := keysFunc.CallMethodArgs("__call__")
	// Convert keys to list to iterate
	builtins, _ := python.ImportModule("builtins")
	limitList := builtins.GetAttr("list").CallMethodArgs("__call__", keysObj)

	length := limitList.Length()
	for i := 0; i < length; i++ {
		key := limitList.GetItem(i)
		val := registry.GetItem(key) // Dict action info

		name := python.AsString(key)
		desc := python.AsString(val.GetItem("description"))
		group := python.AsString(val.GetItem("group"))

		params := make(map[string]interface{})
		pDict := val.GetItem("parameters")

		pKeysObj := pDict.GetAttr("keys").CallMethodArgs("__call__")
		pKeysList := builtins.GetAttr("list").CallMethodArgs("__call__", pKeysObj)
		pLen := pKeysList.Length()

		for j := 0; j < pLen; j++ {
			pK := pKeysList.GetItem(j)
			pV := pDict.GetItem(pK)

			pName := python.AsString(pK)
			pType := python.AsString(pV.GetItem("type"))

			pDefObj := pV.GetItem("default")
			var pDefault interface{}

			// Check None
			if pDefObj.PyObject() != cpy3.Py_None {
				if python.IsInt(pDefObj) {
					pDefault = python.AsInt(pDefObj)
				} else if python.IsString(pDefObj) {
					pDefault = python.AsString(pDefObj)
				} else if python.IsFloat(pDefObj) {
					pDefault = python.AsFloat64(pDefObj)
				} else {
					pDefault = pDefObj.String()
				}
			}
			params[pName] = map[string]interface{}{
				"type":    pType,
				"default": pDefault,
			}
		}

		funcObj := val.GetItem("function")
		// IncRef to keep it alive? GetItem returns new reference or borrowed?
		// High level wrappers often handle refs.
		// python.GetItem returns *Object. NewObject(PyDict_GetItem...) -> Increfs?
		// "NewObject creates a new Object...".
		// If PyDict_GetItem borrows, NewObject likely increments?
		// Let's assume safely held.

		actions[name] = &Action{
			Name:         name,
			Group:        group,
			Description:  desc,
			Parameters:   params,
			TriggerTopic: fmt.Sprintf("tinpot/ops/%s/run", name),
			Function:     funcObj,
		}
		log.Printf("Loaded action: %s", name)
	}
}

func announceActions(c mqtt.Client) {
	actionsMu.RLock()
	defer actionsMu.RUnlock()

	for _, act := range actions {
		topic := fmt.Sprintf("tinpot/actions/%s", act.Name)
		payload, _ := json.Marshal(act)
		c.Publish(topic, 1, true, payload).Wait()
	}
}

func subscribeToActions(c mqtt.Client) {
	actionsMu.RLock()
	defer actionsMu.RUnlock()

	for _, act := range actions {
		topic := act.TriggerTopic
		name := act.Name
		c.Subscribe(topic, 1, func(cl mqtt.Client, msg mqtt.Message) {
			go executeAction(cl, name, msg)
		})
	}
}

type ExecutionRequest struct {
	ExecutionID string                 `json:"execution_id"`
	Parameters  map[string]interface{} `json:"parameters"`
	ResultTopic string                 `json:"result_topic"`
	LogTopic    string                 `json:"log_topic"`
}

func executeAction(c mqtt.Client, actionName string, msg mqtt.Message) {
	execMu.Lock()
	defer execMu.Unlock()

	var req ExecutionRequest
	json.Unmarshal(msg.Payload(), &req)

	stateMu.Lock()
	log.Printf("Executing %s (ID: %s)", actionName, req.ExecutionID)
	currentLogTopic = req.LogTopic
	stateMu.Unlock()

	defer func() {
		stateMu.Lock()
		currentLogTopic = ""
		stateMu.Unlock()
	}()

	actionsMu.RLock()
	act, ok := actions[actionName]
	actionsMu.RUnlock()
	if !ok {
		return
	}

	// Prepare Arguments
	kwargs := cpy3.PyDict_New()
	defer kwargs.DecRef()

	for k, v := range req.Parameters {
		keyStr := cpy3.PyUnicode_FromString(k)
		var valPy *cpy3.PyObject

		switch val := v.(type) {
		case string:
			valPy = cpy3.PyUnicode_FromString(val)
		case float64:
			if float64(int(val)) == val {
				valPy = cpy3.PyLong_FromLong(int(val))
			} else {
				valPy = cpy3.PyFloat_FromDouble(val)
			}
		case bool:
			if val {
				valPy = cpy3.PyBool_FromLong(1)
			} else {
				valPy = cpy3.PyBool_FromLong(0)
			}
		default:
			valPy = cpy3.PyUnicode_FromString(fmt.Sprintf("%v", val))
		}
		cpy3.PyDict_SetItem(kwargs, keyStr, valPy)
		keyStr.DecRef()
		valPy.DecRef()
	}

	argsTuple := cpy3.PyTuple_New(0)
	defer argsTuple.DecRef()

	// Call using cpy3 method
	resPy := act.Function.PyObject().Call(argsTuple, kwargs)

	status := "SUCCESS"
	var result interface{}
	var errMsg string

	if resPy == nil {
		status = "FAILURE"
		if cpy3.PyErr_Occurred() != nil {
			cpy3.PyErr_Print()
			errMsg = "Exception occurred"
		}
	} else {
		// Convert valid result
		defer resPy.DecRef()
		// Check None
		if resPy != cpy3.Py_None {
			// Use json library to dump -> Go Unmarshal
			jsonMod, _ := python.ImportModule("json")
			if jsonMod != nil {
				resObj := python.NewObject(resPy)
				// json.dumps(res)
				jsonStrObj := jsonMod.CallMethodArgs("dumps", resObj)
				if jsonStrObj != nil {
					jsonStr := python.AsString(jsonStrObj)
					json.Unmarshal([]byte(jsonStr), &result)
				} else {
					result = python.AsString(resObj)
				}
			} else {
				// Fallback
				result = "Result check logs"
			}
		}
	}

	resp := map[string]interface{}{
		"status": status,
		"result": result,
		"error":  errMsg,
	}
	pay, _ := json.Marshal(resp)
	c.Publish(req.ResultTopic, 1, true, pay).Wait()
}

func setupLogCapture(c mqtt.Client) {
	r, w, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}
	fd := int(w.Fd())

	script := fmt.Sprintf(`
import sys
import os
sys.stdout = os.fdopen(%d, "w", buffering=1)
sys.stderr = sys.stdout
`, fd)
	cpy3.PyRun_SimpleString(script)

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := r.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}
			lines := strings.Split(string(buf[:n]), "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) == "" {
					continue
				}

				stateMu.Lock()
				topic := currentLogTopic
				stateMu.Unlock()

				if topic != "" {
					entry := map[string]string{
						"timestamp": time.Now().Format(time.RFC3339),
						"level":     "INFO",
						"message":   line,
					}
					d, _ := json.Marshal(entry)
					c.Publish(topic, 0, false, d)
				} else {
					log.Printf("[PY] %s", line)
				}
			}
		}
	}()
}
