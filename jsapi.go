package jsapi

/*
#cgo LDFLAGS: ./lib/libmonk.a -L/home/chrisfarms/src/github.com/chrisfarms/monkey/mozilla-central/js/src/build-release/js/src/ -l:libmozjs-34a1.so
#include <stdlib.h>
#include "lib/js.hpp"
void Init();
*/
import "C"
import (
	"fmt"
	"unsafe"
	"reflect"
	"runtime"
	"encoding/json"
	"sync"
)

func init() {
	C.Init()
}

var contexts = make(map[*C.JSAPIContext]*Context)

type jsapiError int

var(
	ErrRuntimeDestroyed jsapiError = 100
	ErrContextDestroyed jsapiError = 200
	ErrObjectDestroyed jsapiError = 300
	ErrFunctionDestroyed jsapiError = 400
)

type destroyer interface {
	Destroy()
}

func finalizer(x destroyer){
	x.Destroy()
}

func (err jsapiError) Error() string {
	switch err {
	case ErrRuntimeDestroyed:
		return "referenced runtime has been destroyed"
	case ErrContextDestroyed:
		return "referenced context has been destroyed"
	case ErrObjectDestroyed:
		return "referenced module has been destroyed"
	case ErrFunctionDestroyed:
		return "referenced function has been destroyed"
	default:
		return fmt.Sprintf("unknown jsapi error: %d", err)
	}
}

//export callback
func callback(c *C.JSAPIContext, ptr unsafe.Pointer, cname *C.char, args *C.char, argn C.int, out **C.char) C.int {
	cx, ok := contexts[c]
	if !ok {
		*out = C.CString("attempt to use context after destroyed")
		return 0
	}
	name := C.GoString(cname)
	var fn *Func
	if ptr == cx.ptr.o {
		fn, ok = cx.funcs[name]
		if !ok {
			*out = C.CString("attempt to use global func that doesn't appear to exist")
			return 0
		}
	} else {
		o, ok := cx.objs[ptr]
		if !ok {
			*out = C.CString("attempt to use object that doesn't appear to exist")
			return 0
		}
		fn, ok = o.funcs[name]
		if !ok {
			*out = C.CString("attempt to use func that doesn't appear to exist")
			return 0
		}
	}
	json := C.GoStringN(args,argn)
	outjson,err := fn.Call(json)
	if err != nil {
		*out = C.CString(err.Error())
		return 0
	}
	*out = C.CString(outjson)
	return 1
}

//export reporter
func reporter(c *C.JSAPIContext, cfilename *C.char, lineno C.uint, cmsg *C.char) {
	cx, ok := contexts[c]
	if !ok {
		return
	}
	cx.setError(C.GoString(cfilename), uint(lineno), C.GoString(cmsg))
}

type ErrorReport struct {
	Filename string
	Line uint
	Message string
}

func (err *ErrorReport) Error() string {
	return fmt.Sprintf("%s:%d %s", err.Filename, err.Line, err.Message)
}

func (err *ErrorReport) String() string {
	return err.Message
}

type Context struct {
	id int64
	ptr *C.JSAPIContext
	objs map[unsafe.Pointer]*Object
	funcs map[string]*Func
	Valid bool
	errs map[string]*ErrorReport
	mu sync.Mutex
}

// The javascript side ends up calling this when an uncaught
// exception manages to bubble to the top.
func (cx *Context) setError(filename string, line uint, message string) {
	if cx.errs == nil {
		cx.errs = make(map[string]*ErrorReport)
	}
	cx.errs[filename] = &ErrorReport{
		Filename: filename,
		Line: line,
		Message: message,
	}
}

// fetch an error for an eval filename and remove it from the pile
func (cx *Context) getError(filename string) *ErrorReport {
	if err, ok := cx.errs[filename]; ok {
		delete(cx.errs, filename)
		return err
	}
	return nil
}

func (cx *Context) Destroy() {
	if cx.Valid || cx.ptr == nil {
		// lock
		cx.mu.Lock()
		defer cx.mu.Unlock()
		// destroy
		C.JSAPI_DestroyContext(cx.ptr)
		cx.Valid = false
	}
}

// Execute javascript source in Context and discard any response
func (cx *Context) Exec(source string) (err error) {
	if !cx.Valid {
		return ErrContextDestroyed
	}
	// alloc C-strings
	csource := C.CString(source)
	defer C.free(unsafe.Pointer(csource))
	filename := "eval"
	cfilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cfilename))
	// lock
	cx.mu.Lock()
	defer cx.mu.Unlock()
	// eval
	if C.JSAPI_Eval(cx.ptr, csource, cfilename) != C.JSAPI_OK {
		if err := cx.getError(filename); err != nil {
			return err
		}
		return fmt.Errorf("Failed to exec javascript and no error report found")
	}
	return nil
}

// Execute javascript source in Context and scan the response into result.
// Scanning follows the rules of json.Unmarshal so most go native types are
// supported and complex javascript objects can be scanned by referancing structs.
func (cx *Context) Eval(source string, result interface{}) (err error) {
	if !cx.Valid {
		return ErrContextDestroyed
	}
	// alloc C-string
	csource := C.CString(source)
	defer C.free(unsafe.Pointer(csource))
	var jsonData *C.char
	var jsonLen C.int
	filename := "eval"
	cfilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cfilename))
	// lock
	cx.mu.Lock()
	defer cx.mu.Unlock()
	// eval
	if C.JSAPI_EvalJSON(cx.ptr, csource, cfilename, &jsonData, &jsonLen) != C.JSAPI_OK {
		if err := cx.getError(filename); err != nil {
			return err
		}
		return fmt.Errorf("Failed to eval javascript and no error report found")
	}
	defer C.free(unsafe.Pointer(jsonData))
	// convert to go
	b := []byte(C.GoStringN(jsonData, jsonLen))
	err = json.Unmarshal(b, result)
	if err != nil {
		return err
	}
	return nil
}

// Define a javascript object in this Context
func (cx *Context) DefineObject(name string) *Object {
	o := NewObject()
	o.cx = cx
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	// lock
	cx.mu.Lock()
	defer cx.mu.Unlock()
	// define
	o.ptr = C.JSAPI_DefineObject(cx.ptr, nil, cname)
	cx.objs[o.ptr] = o
	return o
}

func (cx *Context) DefineFunction(name string, fun interface{}) *Func {
	f := NewFunc(fun)
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	// lock
	cx.mu.Lock()
	defer cx.mu.Unlock()
	// define
	C.JSAPI_DefineFunction(cx.ptr, nil, cname)
	cx.funcs[name] = f
	f.Name = name
	return f
}

func NewContext() *Context {
	cx := &Context{}
	// lock
	cx.mu.Lock()
	defer cx.mu.Unlock()
	// create
	cx.ptr = C.JSAPI_NewContext()
	cx.Valid = true
	cx.objs = make(map[unsafe.Pointer]*Object)
	cx.funcs = make(map[string]*Func)
	contexts[cx.ptr] = cx
	runtime.SetFinalizer(cx, finalizer)
	return cx
}

type Object struct {
	id int64
	cx *Context
	ptr unsafe.Pointer
	funcs map[string]*Func
}

func NewObject() *Object {
	o := &Object{}
	o.funcs = make(map[string]*Func)
	return o
}

func (o *Object) DefineFunction(name string, fun interface{}) *Func {
	f := NewFunc(fun)
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	// lock
	o.cx.mu.Lock()
	defer o.cx.mu.Unlock()
	// define
	C.JSAPI_DefineFunction(o.cx.ptr, o.ptr, cname)
	o.funcs[name] = f
	f.Name = name
	return f
}

type Func struct {
	Name string
	v reflect.Value
	t reflect.Type
}

func NewFunc(fun interface{}) *Func {
	f := &Func{}
	f.v = reflect.ValueOf(fun)
	if !f.v.IsValid() {
		panic("invalid function type")
	}
	f.t = f.v.Type()
	if f.t.Kind() != reflect.Func {
		panic("X is not a valid function type")
	}
	// check inarg types are acceptable
	for i := 0; i < f.t.NumIn(); i++ {
		switch f.t.In(i).Kind() {
		case reflect.Bool,reflect.Int,reflect.Int8,reflect.Int16,
			reflect.Int32,reflect.Int64,reflect.Uint,reflect.Uint8,
			reflect.Uint16,reflect.Uint32,reflect.Uint64,reflect.Float32,
			reflect.Float64,reflect.Interface,reflect.Map,reflect.Slice,
			reflect.String:
			// ok
		default:
			panic("X is not a valid argument type for javascript interop")
		}
	}
	f.Name = "[anon]"
	return f
}

func (f *Func) Call(in string) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s: %v", f.Name, r)
		}
	}()
	return f.call(in)
}

func (f *Func) call(in string) (out string, err error) {
	// decode args
	var inargs []interface{}
	err = json.Unmarshal([]byte(in), &inargs)
	if err != nil {
		return
	}
	// validate args
	if len(inargs) != f.t.NumIn() && !f.t.IsVariadic() {
		return "", fmt.Errorf("Invalid number of arguments: expected %d got %d", f.t.NumIn(), len(inargs))
	}
	invals := make([]reflect.Value, len(inargs))
	for i := 0; i < len(inargs); i++ {
		v := reflect.ValueOf(inargs[i])
		var t reflect.Type
		if f.t.IsVariadic() && i >= f.t.NumIn()-1 { // handle varargs
			t = f.t.In(f.t.NumIn()-1).Elem()
		} else {
			t = f.t.In(i)
		}
		if v.Type().Kind() == reflect.Ptr && t.Kind() != reflect.Ptr {
			v = reflect.Indirect(v)
		}
		if !v.Type().AssignableTo(t) {
			if !v.Type().ConvertibleTo(t) {
				return "", fmt.Errorf("Invalid argument type: arg[%d] should be type %s but got %s", i, t.Kind(), v.Type().Kind())
			}
			v = v.Convert(t)
		}
		invals[i] = v
	}
	// call func
	outvals := f.v.Call(invals)
	switch len(outvals) {
	case 0:
		return "", nil
	case 1:
		b,err := json.Marshal(outvals[0].Interface())
		return string(b), err
	default:
		outargs := make([]interface{}, len(outvals))
		for i := 0; i < len(outvals); i++ {
			outargs[i] = outvals[i].Interface()
		}
		b,err := json.Marshal(outargs)
		return string(b), err
	}
}
