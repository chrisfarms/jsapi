// Package jsapi enables embedding of the spidermonkey javascript engine within Go projects.
package jsapi

/*
#cgo LDFLAGS: -L./lib -L./src/github.com/jsapi/lib -L./src/github.com/jaspi/lib/moz/js/src/build-release/dist/lib -L./lib/moz/js/src/build-release/dist/lib -ljsapi -l:libjs.a -lpthread -lstdc++ -ldl -l:libnspr4.a
#include <stdlib.h>
#include "lib/js.hpp"
void Init();
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"runtime"
	"unsafe"
)

// Raw is a special type that can be returned by defined
// functions to return raw javascript/json to be interpreted.
// This can be useful if your function actually returns JSON
type Raw string

var jsapi *api

type fn struct {
	call func()
	done chan bool
}

type cxfn struct {
	call func(*C.JSAPIContext)
	done chan bool
}

type api struct {
	in chan *fn
}

func (jsapi *api) do(callback func()) {
	if C.JSAPI_ThreadCanAccessRuntime() == C.JSAPI_OK {
		callback()
		return
	}
	fn := &fn{
		call: callback,
		done: make(chan bool, 1),
	}
	jsapi.in <- fn
	<-fn.done
}

func start() *api {
	jsapi := &api{
		in: make(chan *fn),
	}
	ready := make(chan bool)
	go func() {
		runtime.LockOSThread()
		C.Init()
		if C.JSAPI_Init() != C.JSAPI_OK {
			panic("could not init JSAPI runtime")
		}
		ready <- true
		for {
			select {
			case fn := <-jsapi.in:
				fn.call()
				fn.done <- true
			}
		}

	}()
	<-ready
	return jsapi
}

func init() {
	jsapi = start()
}

var contexts = make(map[int]*Context)

type destroyer interface {
	Destroy()
}

func finalizer(x destroyer) {
	x.Destroy()
}

//export workerWait
func workerWait(id C.int, ptr *C.JSAPIContext) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cx, ok := contexts[int(id)]
	if !ok {
		panic("attempt to wait on a non existant context worker")
	}
	cx.ready <- nil
	for {
		select {
		case fn, ok := <-cx.in:
			if !ok {
				return
			}
			fn.call(ptr)
			fn.done <- true
		}
	}
}

//export workerFail
func workerFail(id C.int, err *C.char) {
	cx, ok := contexts[int(id)]
	if !ok {
		panic("attempt to wait on a non existant context worker")
	}
	cx.ready <- fmt.Errorf("worker %d: %s", int(id), C.GoString(err))
}

//export callFunction
func callFunction(c *C.JSAPIContext, fid C.uint32_t, cname *C.char, args *C.char, argn C.int, out **C.char) C.int {
	name := C.GoString(cname)
	cx, ok := contexts[int(c.id)]
	if !ok {
		*out = C.CString(fmt.Sprintf("attempt to call function %s on a destroyed context", name))
		return 0
	}
	fn, ok := cx.funcs[int(fid)]
	if !ok {
		*out = C.CString(fmt.Sprintf("attempt to call function %s that doesn't appear to exist in context", name))
		return 0
	}
	json := C.GoStringN(args, argn)
	outjson, err := fn.call(json)
	if err != nil {
		*out = C.CString(err.Error())
		return 0
	}
	*out = C.CString(outjson)
	return 1
}

//export reporter
func reporter(c *C.JSAPIContext, cfilename *C.char, lineno C.uint, cmsg *C.char) {
	cx, ok := contexts[int(c.id)]
	if !ok {
		return
	}
	cx.setError(C.GoString(cfilename), uint(lineno), C.GoString(cmsg))
}

//export getprop
func getprop(c *C.JSAPIContext, id C.uint32_t, cname *C.char, out **C.char) C.int {
	cx, ok := contexts[int(c.id)]
	if !ok {
		*out = C.CString("attempt to use context after destroyed")
		return 0
	}
	o, ok := cx.objs[int(id)]
	if !ok {
		fmt.Println("bad object id", id)
		*out = C.CString("attempt to use object that doesn't appear to exist")
		return 0
	}
	p, ok := o.props[C.GoString(cname)]
	if !ok {
		*out = C.CString("attempt to get property that doesn't appear to exist")
		return 0
	}
	outjson, err := p.get()
	if err != nil {
		*out = C.CString(err.Error())
		return 0
	}
	*out = C.CString(outjson)
	return 1
}

//export setprop
func setprop(c *C.JSAPIContext, id C.uint32_t, cname *C.char, val *C.char, valn C.int, out **C.char) C.int {
	cx, ok := contexts[int(c.id)]
	if !ok {
		*out = C.CString("attempt to use context after destroyed")
		return 0
	}
	o, ok := cx.objs[int(id)]
	if !ok {
		*out = C.CString("attempt to use object that doesn't appear to exist")
		return 0
	}
	p, ok := o.props[C.GoString(cname)]
	if !ok {
		*out = C.CString("attempt to set property that doesn't appear to exist")
		return 0
	}
	json := C.GoStringN(val, valn)
	outjson, err := p.set(json)
	if err != nil {
		*out = C.CString(err.Error())
		return 0
	}
	*out = C.CString(outjson)
	return 1
}

type ErrorReport struct {
	Filename string
	Line     uint
	Message  string
}

func (err *ErrorReport) Error() string {
	return fmt.Sprintf("%s:%d %s", err.Filename, err.Line, err.Message)
}

func (err *ErrorReport) String() string {
	return err.Message
}

// Types that implement Definer can create mappings of objects
// and functions between javascript and Go
type Definer interface {
	DefineFunction(name string, fun interface{}) error
	DefineObject(name string, proxy interface{}) (Definer, error)
}

// Types that impliment Evaluator can execute javascript
type Evaluator interface {
	Exec(source string) (err error)
	Eval(source string, result interface{}) (err error)
	ExecFile(filename string) (err error)
	ExecFrom(r io.Reader) (err error)
}

// Context is a javascript runtime environment and global namespace. You run javascript
// _within_ a context. You can think of it a bit like a tab in a browser, scripts
// running in seperate Contexts cannot see or interact with each other.
//
// Each Context runs in it's own thread, and setup/teardown of Contexts is not free so
// for best results you should consider how to make best use of the Contexts and try to
// keep the number you need to a minimum.
type Context struct {
	id    int
	ptr   *C.JSAPIContext
	ready chan error
	in    chan *cxfn
	objs  map[int]*Object
	funcs map[int]*function
	Valid bool
	errs  map[string]*ErrorReport
}

// Create a context to execute javascript in.
func NewContext() *Context {
	cx := &Context{}
	cx.id = uid()
	cx.ready = make(chan error, 1)
	cx.in = make(chan *cxfn)
	cx.objs = make(map[int]*Object)
	cx.funcs = make(map[int]*function)
	var err error
	jsapi.do(func() {
		if C.JSAPI_NewContext(C.int(cx.id)) != C.JSAPI_OK {
			err = fmt.Errorf("failed to spawn new context")
			return
		}
		contexts[cx.id] = cx
	})
	if err != nil {
		panic(err)
	}
	err = <-cx.ready
	if err != nil {
		panic(err)
	}
	cx.Valid = true
	runtime.SetFinalizer(cx, finalizer)
	cx.do(func(ptr *C.JSAPIContext) {
		cx.ptr = ptr
	})
	return cx
}

// The javascript side ends up calling this when an uncaught
// exception manages to bubble to the top.
func (cx *Context) setError(filename string, line uint, message string) {
	if cx.errs == nil {
		cx.errs = make(map[string]*ErrorReport)
	}
	cx.errs[filename] = &ErrorReport{
		Filename: filename,
		Line:     line,
		Message:  message,
	}
}

// fetch an error for an eval filename and remove it from the pile
func (cx *Context) getError(filename string) *ErrorReport {
	if err, ok := cx.errs[filename]; ok {
		delete(cx.errs, filename)
		return err
	}
	if err, ok := cx.errs["__fatal__"]; ok {
		delete(cx.errs, filename)
		return err
	}
	return nil
}

// Teardown the context. It is an error to use a context after
// it is destroyed.
func (cx *Context) Destroy() {
	if cx.Valid {
		close(cx.in)
		cx.Valid = false
	}
}

// Execute javascript source in Context and discard any response
func (cx *Context) Exec(source string) (err error) {
	return cx.exec(source, "exec")
}

func (cx *Context) exec(source string, filename string) (err error) {
	cx.do(func(ptr *C.JSAPIContext) {
		csource := C.CString(source)
		defer C.free(unsafe.Pointer(csource))
		cfilename := C.CString(filename)
		defer C.free(unsafe.Pointer(cfilename))
		// eval
		if C.JSAPI_Eval(ptr, csource, cfilename) != C.JSAPI_OK {
			if err = cx.getError(filename); err != nil {
				return
			}
			err = fmt.Errorf("Failed to exec javascript and no error report found")
			return
		}
	})
	return err
}

// Execute javascript source in Context and scan the response into result.
// Scanning follows the rules of json.Unmarshal so most go native types are
// supported and complex javascript objects can be scanned by referancing structs.
// The special jsapi.Raw string type can be used if you just the output as a JSON
// string.
func (cx *Context) Eval(source string, result interface{}) (err error) {
	cx.do(func(ptr *C.JSAPIContext) {
		// alloc C-string
		csource := C.CString(source)
		defer C.free(unsafe.Pointer(csource))
		var jsonData *C.char
		var jsonLen C.int
		filename := "eval"
		cfilename := C.CString(filename)
		defer C.free(unsafe.Pointer(cfilename))
		// eval
		if C.JSAPI_EvalJSON(ptr, csource, cfilename, &jsonData, &jsonLen) != C.JSAPI_OK {
			if err = cx.getError(filename); err != nil {
				return
			}
			err = fmt.Errorf("Failed to eval javascript and no error report found")
			return
		}
		defer C.free(unsafe.Pointer(jsonData))
		// convert to go
		b := []byte(C.GoStringN(jsonData, jsonLen))
		if raw, ok := result.(*Raw); ok {
			*raw = Raw(string(b))
		} else {
			err = json.Unmarshal(b, result)
		}
	})
	return err
}

// Execute javascript in the context from an io.Reader.
func (cx *Context) ExecFrom(r io.Reader) (err error) {
	return cx.execFrom(r, "ExecFrom")
}

func (cx *Context) execFrom(r io.Reader, filename string) (err error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return
	}
	return cx.exec(string(b), filename)
}

// Execute javascript in the context from a file
func (cx *Context) ExecFile(filename string) (err error) {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return cx.execFrom(f, filename)
}

// Define a javascript object in the Context.
// If proxy is nil, then an empty js object is created.
// If proxy references a struct type, then a two-way binding of all public
// fields within proxy the proxy object will be exposed to js via the
// created object.
func (cx *Context) DefineObject(name string, proxy interface{}) (Definer, error) {
	return cx.defineObject(name, proxy, 0)
}

func (cx *Context) defineObject(name string, proxy interface{}, id int) (o *Object, err error) {
	o = &Object{}
	o.props = make(map[string]*prop)
	o.cx = cx
	o.id = uid()
	cx.do(func(ptr *C.JSAPIContext) {
		cname := C.CString(name)
		defer C.free(unsafe.Pointer(cname))
		if C.JSAPI_DefineObject(ptr, C.uint32_t(id), cname, C.uint32_t(o.id)) != C.JSAPI_OK {
			err = fmt.Errorf("failed to define object")
			return
		}
		if proxy != nil {
			o.proxy = proxy
			ov := reflect.ValueOf(proxy)
			ot := ov.Type()
			if ot.Kind() == reflect.Ptr {
				ov = reflect.Indirect(ov)
				ot = ov.Type()
			}
			if ot.Kind() != reflect.Struct {
				err = fmt.Errorf("proxy object must be a kind of struct or pointer to a struct")
				return
			}
			for i := 0; i < ot.NumField(); i++ {
				f := ot.Field(i)
				fv := ov.Field(i)
				o.props[f.Name] = &prop{f.Name, fv, f.Type}
				cpropname := C.CString(f.Name)
				defer C.free(unsafe.Pointer(cpropname))
				if C.JSAPI_DefineProperty(ptr, C.uint32_t(o.id), cpropname) != C.JSAPI_OK {
					err = fmt.Errorf("failed to define property")
					return
				}
			}
		}
		cx.objs[o.id] = o
	})
	return
}

func (cx *Context) DefineFunction(name string, fun interface{}) error {
	return cx.defineFunction(name, fun, 0)
}

func (cx *Context) defineFunction(name string, fun interface{}, parent int) (err error) {
	f := &function{}
	f.id = uid()
	f.v = reflect.ValueOf(fun)
	if !f.v.IsValid() {
		return fmt.Errorf("invalid function type")
	}
	f.t = f.v.Type()
	if f.t.Kind() != reflect.Func {
		return fmt.Errorf("not a valid function type")
	}
	f.name = "[anon]"
	cx.do(func(ptr *C.JSAPIContext) {
		cname := C.CString(name)
		defer C.free(unsafe.Pointer(cname))
		if C.JSAPI_DefineFunction(ptr, C.uint32_t(parent), cname, C.uint32_t(f.id)) != C.JSAPI_OK {
			err = fmt.Errorf("failed to define function")
			return
		}
		f.name = name
		cx.funcs[f.id] = f
	})
	return
}

// Attempt to aquire mutex, then runs in primary thread.
// panics if Context is invalid
func (cx *Context) do(callback func(*C.JSAPIContext)) {
	if !cx.Valid {
		panic("attempt to use a destroyed context")
	}
	if cx.ptr != nil && C.JSAPI_ThreadCanAccessContext(cx.ptr) == C.JSAPI_OK {
		callback(cx.ptr)
		return
	}
	fn := &cxfn{
		call: callback,
		done: make(chan bool, 1),
	}
	cx.in <- fn
	<-fn.done
}

type Object struct {
	id    int
	cx    *Context
	props map[string]*prop
	proxy interface{}
}

func (o *Object) DefineFunction(name string, fun interface{}) error {
	return o.cx.defineFunction(name, fun, o.id)
}

func (o *Object) DefineObject(name string, proxy interface{}) (Definer, error) {
	return o.cx.defineObject(name, proxy, o.id)
}

type function struct {
	id   int
	name string
	v    reflect.Value
	t    reflect.Type
}

func (f *function) call(in string) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s: %v", f.name, r)
		}
	}()
	return f.rawcall(in)
}

func (f *function) rawcall(in string) (out string, err error) {
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
			t = f.t.In(f.t.NumIn() - 1).Elem()
		} else {
			t = f.t.In(i)
		}
		v, err = cast(v, t)
		if err != nil {
			return
		}
		invals[i] = v
	}
	// call func
	outvals := f.v.Call(invals)
	if len(outvals) > 1 {
		panic("javascript does not support multiple return params")
	}
	if len(outvals) == 0 {
		return "", nil
	}
	outv := outvals[0].Interface()
	if raw, ok := outv.(Raw); ok {
		return string(raw), nil
	}
	b, err := json.Marshal(outv)
	return string(b), err
}

// try to convert v to something that is assignable to type t
func cast(v reflect.Value, t reflect.Type) (reflect.Value, error) {
	if v.Type().Kind() == reflect.Ptr && t.Kind() != reflect.Ptr {
		v = v.Elem()
	}
	if !v.Type().AssignableTo(t) {
		if !v.Type().ConvertibleTo(t) {
			// A common failure here is that we want to
			// cast a map[string]interface{} -> struct.
			// This is obviously not possible, but since it is *known*
			// that the source and target can both be serialized to
			// JSON, we can do a nasty hack to fix it
			// TODO: find a better way!
			if (t.Kind() == reflect.Ptr || t.Kind() == reflect.Struct) && v.Type().Kind() == reflect.Map {
				b, err := json.Marshal(v.Interface())
				if err != nil {
					return v, fmt.Errorf("cannot cast %s to %s: %s", v.Type().Kind(), t.Kind(), err.Error())
				}
				vt := t
				if vt.Kind() == reflect.Ptr {
					vt = t.Elem()
				}
				vv := reflect.New(vt)
				err = json.Unmarshal(b, vv.Interface())
				if err != nil {
					return v, fmt.Errorf("cannot cast %s to %s: %s", v.Type().Kind(), t.Kind(), err.Error())
				}
				if t.Kind() != reflect.Ptr && vv.Type().Kind() == reflect.Ptr {
					vv = vv.Elem()
				}
				return vv, nil
			}
			return v, fmt.Errorf("cannot cast %s to %s", v.Type().Kind(), t.Kind())
		}
		v = v.Convert(t)
	}
	return v, nil
}

// prop is a wrapper around a struct's field's refelction
type prop struct {
	name string
	v    reflect.Value
	t    reflect.Type
}

// get json for property
func (p *prop) get() (string, error) {
	b, err := json.Marshal(p.v.Interface())
	return string(b), err
}

// set property via json
func (p *prop) set(injson string) (string, error) {
	var x interface{}
	err := json.Unmarshal([]byte(injson), &x)
	if err != nil {
		return "", err
	}
	xv := reflect.ValueOf(x)
	xv, err = cast(xv, p.t)
	if !p.v.CanSet() {
		return "", fmt.Errorf("property %s is not settable", p.name)
	}
	p.v.Set(xv)
	return p.get()
}
