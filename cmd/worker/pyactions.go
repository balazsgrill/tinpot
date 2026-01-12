package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/balazsgrill/tinpot"
	cpy3 "go.nhat.io/cpy/v3"
	"go.nhat.io/python/v3"
)

type Action struct {
	Name        string                          `json:"name"`
	Group       string                          `json:"group"`
	Description string                          `json:"description"`
	Parameters  map[string]tinpot.ParameterInfo `json:"parameters"`
	//TriggerTopic string                 `json:"trigger_topic"`
	Function *python.Object `json:"-"`
}

type pyActionInfo struct {
	tinpot.ActionInfo
	Function *python.Object
}

type pyActionManager struct {
	actions         map[string]*pyActionInfo
	actionsMu       sync.RWMutex
	execMu          sync.Mutex
	stateMu         sync.Mutex
	currentLogTopic string
	mainThreadState *cpy3.PyThreadState
}

func (act *pyActionInfo) trigger(parameters map[string]interface{}, response tinpot.ActionResponse, logs tinpot.ActionLogs) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	w := setupLogCapture(logs)
	defer w.Close()

	// Acquire GIL
	gstate := cpy3.PyGILState_Ensure()
	defer cpy3.PyGILState_Release(gstate)

	// Prepare Arguments
	kwargs := cpy3.PyDict_New()
	defer kwargs.DecRef()

	for k, v := range parameters {
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
	if argsTuple == nil {
		log.Printf("ERROR: PyTuple_New failed")
		response("Internal Error", nil)
		return
	}
	defer argsTuple.DecRef()

	log.Printf("Triggering action %s (argsTuple=%p, kwargs=%p)", act.Name, argsTuple, kwargs)

	// Call using cpy3 method
	resPy := act.Function.PyObject().Call(argsTuple, kwargs)
	log.Printf("Python call returned %p", resPy)

	var result map[string]interface{}
	var errMsg string

	if resPy == nil {
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
					result = map[string]interface{}{"result": python.AsString(resObj)}
				}
			} else {
				result = nil
			}
		}
	}
	log.Printf("Trigger finished, sending result")
	response(errMsg, result)
}

func setupLogCapture(callback tinpot.ActionLogs) *os.File {
	r, w, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}
	fd := int(w.Fd())

	script := fmt.Sprintf(`
import sys
import os
sys.stdout = os.fdopen(%d, "w", buffering=1, closefd=False)
sys.stderr = sys.stdout
`, fd)
	// Run with GIL
	gstate := cpy3.PyGILState_Ensure()
	cpy3.PyRun_SimpleString(script)
	cpy3.PyGILState_Release(gstate)

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

				callback("INFO", line)
			}
		}
	}()
	return w
}

func setupPython() {
	sys, err := python.ImportModule("sys")
	if err != nil {
		log.Fatal(err)
	}
	path := sys.GetAttr("path")

	cwd, _ := os.Getwd()

	// Extract embedded lib to temp directory
	libPath, err := extractEmbeddedLib()
	if err != nil {
		log.Fatalf("Failed to extract embedded lib: %v", err)
	}
	log.Printf("Extracted embedded lib to: %s", libPath)

	// Add temp lib path to python sys.path
	// Also add ActionsDir so actions can be found

	// Prepend libPath to ensure it takes precedence
	path.CallMethodArgs("insert", 0, libPath)
	path.CallMethodArgs("append", cwd)
	path.CallMethodArgs("append", ActionsDir)
}

func (mgr *pyActionManager) discoverActions() {
	mgr.actionsMu.Lock()
	defer mgr.actionsMu.Unlock()
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

		params := make(map[string]tinpot.ParameterInfo)
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
			params[pName] = tinpot.ParameterInfo{
				Type:    pType,
				Default: pDefault,
			}
		}

		funcObj := val.GetItem("function")
		// IncRef to keep it alive? GetItem returns new reference or borrowed?
		// High level wrappers often handle refs.
		// python.GetItem returns *Object. NewObject(PyDict_GetItem...) -> Increfs?
		// "NewObject creates a new Object...".
		// If PyDict_GetItem borrows, NewObject likely increments?
		// Let's assume safely held.

		mgr.actions[name] = &pyActionInfo{
			ActionInfo: tinpot.ActionInfo{
				Name:        name,
				Group:       group,
				Description: desc,
				Parameters:  params,
			},
			Function: funcObj,
		}
		log.Printf("Loaded action: %s", name)
	}
}

func NewPyActionManager() tinpot.ActionManager {
	// Initialize Python
	cpy3.Py_Initialize()

	setupPython()

	// Release GIL to allow other threads to run
	result := &pyActionManager{
		actions: make(map[string]*pyActionInfo),
	}

	result.discoverActions()

	// Release GIL to allow other threads to run
	result.mainThreadState = cpy3.PyEval_SaveThread()

	return result
}

func (mgr *pyActionManager) GetAction(name string) tinpot.ActionTrigger {
	mgr.actionsMu.RLock()
	defer mgr.actionsMu.RUnlock()

	action, ok := mgr.actions[name]
	if !ok {
		return nil
	}
	return action.trigger
}

func (mgr *pyActionManager) ListActions() map[string]tinpot.ActionInfo {
	mgr.actionsMu.RLock()
	defer mgr.actionsMu.RUnlock()

	result := make(map[string]tinpot.ActionInfo)
	for name, act := range mgr.actions {
		result[name] = act.ActionInfo
	}
	return result
}

func (mgr *pyActionManager) IsConnected() bool {
	return true
}
