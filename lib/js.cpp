#include "mozilla/ArrayUtils.h"
#include "mozilla/Atomics.h"
#include "mozilla/DebugOnly.h"
#include "mozilla/GuardObjects.h"
#include "mozilla/PodOperations.h"

#ifdef XP_WIN
# include <direct.h>
# include <process.h>
#endif
#include <errno.h>
#include <fcntl.h>
#if defined(XP_WIN)
# include <io.h>     /* for isatty() */
#endif
#include <locale.h>
#include <math.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#ifdef XP_UNIX
# include <sys/mman.h>
# include <sys/stat.h>
# include <sys/wait.h>
# include <unistd.h>
#endif


#include "jsapi.h"
#include "jsarray.h"
#include "jsatom.h"
#include "jsobj.h"
#include "jsprf.h"
#include "jstypes.h"
#include "jsutil.h"
#include "jswrapper.h"
#include "prmjtime.h"

#include "builtin/TestingFunctions.h"
#include "js/StructuredClone.h"
#include "perf/jsperf.h"
#include "shell/jsheaptools.h"
#include "shell/jsoptparse.h"
#include "vm/ArgumentsObject.h"
#include "vm/Shape.h"
#include "vm/WrapperObject.h"
#include "vm/HelperThreads.h"

struct JSAPIContext {
	JSRuntime *rt;
	JSContext *cx;
	JSObject *o;
	JSObject *objs;
	int id;
};

#include "js.hpp"
#include <sstream>

using namespace js;
using namespace JS;

using mozilla::ArrayLength;
using mozilla::MakeUnique;
using mozilla::Maybe;
using mozilla::NumberEqualsInt32;
using mozilla::PodCopy;
using mozilla::PodEqual;
using mozilla::UniquePtr;

#define OBJECT_ID_KEY "__oid__"


/* The class of the global object. */
static const JSClass global_class = {
    "global", JSCLASS_NEW_RESOLVE | JSCLASS_GLOBAL_FLAGS | JSCLASS_HAS_PRIVATE,
    JS_PropertyStub,  JS_DeletePropertyStub,
    JS_PropertyStub,  JS_StrictPropertyStub,
    JS_EnumerateStub, JS_ResolveStub,
    JS_ConvertStub, nullptr,
	nullptr, nullptr, nullptr,
	JS_GlobalObjectTraceHook
};


// The error reporter callback.
void reportError(JSContext *cx, const char *message, JSErrorReport *report) {
	JSAPIContext *c = (JSAPIContext*)JS_GetContextPrivate(cx);
	go_error(c, (char*)report->filename, (unsigned int)report->lineno, (char*)message);
}

// The OOM reporter
void reportOOM(JSContext *cx, void *data) {
	JSAPIContext *c = (JSAPIContext*)data;
	fprintf(stderr, "spidermonkey has run out of memory!\n");
	go_error(c, "__fatal__", 0, "spidermonkey ran out of memory"); 
}

JSObject* idToObj(JSAPIContext* c, uint32_t id){
	JSAutoRequest ar(c->cx);
	JSAutoCompartment ac(c->cx, c->o);
	RootedObject objs(c->cx, c->objs);
	// lookup parent using global key
	// (this is a bit hacky) but it's the most stable way I've found
	// to pass ids around that also handles GC perfectly
	RootedValue v(c->cx);
	if (!JS_GetElement(c->cx, objs, id, &v) ){
		fprintf(stderr, "fatal: could not lookup object for id %d", id);
		return NULL;
	}
	if( v.isNull() || v.isUndefined() ){
		fprintf(stderr, "fatal: could not find object for id %d\n", id);
		return 0;
	}
	if( !v.isObject() ){
		fprintf(stderr, "fatal: object lookup for id %d returned unexpected type\n", id);
		return 0;
	}
	return &v.toObject();
}

uint32_t objId(JSAPIContext* c, HandleObject obj){
	JSAutoRequest ar(c->cx);
	JSAutoCompartment ac(c->cx, c->o);
	RootedValue idval(c->cx);
	if( !JS_GetProperty(c->cx, obj, OBJECT_ID_KEY, &idval) ){
		fprintf(stderr, "fatal: failed to lookup object's id\n");
		return 0;
	}
	if( idval.isNull() || idval.isUndefined() ){
		fprintf(stderr, "fatal: object does not have an id\n");
		return 0;
	}
	return idval.toInt32();
}

jerr JSAPI_DefineObject(JSAPIContext *c, uint32_t pid, char* name, uint32_t id){
	JSAutoRequest ar(c->cx);
	JSAutoCompartment ac(c->cx, c->o);
	RootedObject p(c->cx, idToObj(c, pid));
	// create object
	RootedObject obj(c->cx, JS_DefineObject(c->cx, p, name, nullptr, JS::NullPtr(), 0));
	bool ok = JS_DefineProperty(
			c->cx,           // context
			obj,             // prop's owner
			OBJECT_ID_KEY,   // prop name
			id,              // initial value
			JSPROP_READONLY, // flags
			nullptr,         // getter callback
			nullptr          // setter callback
	);
	if( !ok ){
		return JSAPI_FAIL;
	}
	// assign to global hash
	RootedObject h(c->cx, c->objs);
	if (!JS_SetElement(c->cx, h, id, obj) ){
		return JSAPI_FAIL;
	}
	return JSAPI_OK;
}

struct jsonBuffer {
	JSContext* cx;
	JSObject* o;
	char* str;
	uint32_t n;
};

bool stringifier(const jschar *s, uint32_t n, void *data){
	jsonBuffer *buf = (jsonBuffer*)data;
    JSAutoRequest ar(buf->cx);
	JSAutoCompartment ac(buf->cx, buf->o);
	RootedString ss(buf->cx,JS_NewUCStringCopyN(buf->cx, s, n));
	size_t sn = JS_GetStringEncodingLength(buf->cx, ss);
	buf->str = (char*)realloc(buf->str, (buf->n + sn) * sizeof(char));
	if( buf->str == NULL){
		printf("could not realloc during stringify");
		return false;
	}
	buf->n = JS_EncodeStringToBuffer(buf->cx, ss, buf->str+buf->n, sn) + buf->n;
	return true;
}

bool wrapGoFunction(JSContext *cx, unsigned argc, JS::Value *vp) {
	JSAPIContext *c = (JSAPIContext*)JS_GetContextPrivate(cx);
	JSAutoRequest ar(c->cx);
	JSAutoCompartment ac(c->cx, c->o);
	// get name
	JS::CallReceiver rec = JS::CallReceiverFromVp(vp);
	RootedObject callee(cx, &rec.callee());
	RootedValue nameval(cx);
	if( !JS_GetProperty(c->cx, callee, "name", &nameval) ){
		fprintf(stderr, "could not find callee name");
		return false;
	}
	RootedValue out(cx);
	RootedString namestr(c->cx, ToString(c->cx, nameval));
    JSAutoByteString bytes;
	char *name = bytes.encodeUtf8(c->cx, namestr); 
	// get args
	JS::CallArgs args = JS::CallArgsFromVp(argc, vp);
	RootedObject argArray(c->cx, JS_NewArrayObject(c->cx, argc));
	for(int i = 0; i<argc; i++){
		JS_DefineElement(c->cx, argArray, i, args[i], 0, nullptr, nullptr);
	}
	RootedValue argValues(c->cx, OBJECT_TO_JSVAL(argArray));
	// convert to json 
	jsonBuffer buf;
	buf.str = NULL;
	buf.cx = c->cx;
	buf.o = c->o;
	buf.n = 0;
	RootedObject replacer(c->cx);
	RootedValue undefined(c->cx);
	JS_Stringify(c->cx, &argValues, replacer, undefined, stringifier, &buf);
	// send to go and parse resulting json
	bool ok = true;
	char *result = NULL;
	if( go_callback(c, objId(c, callee), name, buf.str, int(buf.n), &result) ){
		if( strlen(result) > 0 ){
			RootedString resultstr(c->cx, JS_NewStringCopyZ(c->cx, result));
			if( JS_ParseJSON(c->cx, resultstr, &out) ){
				args.rval().set(out);
			} else {
				ok = false;
			}
		} else { // return undefined if no json but healthy response
			args.rval().setUndefined();
		}
	} else {
		ok = false;
		JS_ReportError(c->cx, "%s", result);
	}
	// Freeeeeeeee
	if( result != NULL ){
		free(result);
	}
	free(buf.str);
	return ok;
}

jerr JSAPI_DefineFunction(JSAPIContext *c, uint32_t pid, char* name, uint32_t fid){
	JSAutoRequest ar(c->cx);
	JSAutoCompartment ac(c->cx, c->o);
	RootedObject p(c->cx, idToObj(c, pid));
	RootedObject fun(c->cx, JS_DefineFunction(c->cx, p, name, wrapGoFunction, 0, 0));
	bool ok = JS_DefineProperty(
			c->cx,           // context
			fun,             // prop's owner
			OBJECT_ID_KEY,   // prop name
			fid,             // initial value
			JSPROP_READONLY, // flags
			nullptr,         // getter callback
			nullptr          // setter callback
	);
	if( !ok ){
		return JSAPI_FAIL;
	}
	return JSAPI_OK;
}

bool wrapGoGetter(JSContext *cx,  JS::HandleObject obj, JS::Handle<jsid> propid, JS::MutableHandle<JS::Value> vp) {
	JSAPIContext *c = (JSAPIContext*)JS_GetContextPrivate(cx);
	JSAutoRequest ar(cx);
	JSAutoCompartment ac(cx, c->o);
	// get property name
    RootedValue idvalue(cx, IdToValue(propid));
    RootedString idstring(cx, ToString(cx, idvalue));
    JSAutoByteString idstr;
    if (!idstr.encodeLatin1(cx, idstring)){
		JS_ReportError(c->cx, "%s", "property id was not a valid string");
        return false;
	}
	// call go
	bool ok = true;
	char* result = NULL;
	RootedValue out(cx);
	if( go_getter(c, objId(c, obj), idstr.ptr(), &result) ){
		if( strlen(result) > 0 ){
			RootedString resultstr(c->cx, JS_NewStringCopyZ(c->cx, result));
			if( JS_ParseJSON(c->cx, resultstr, &out) ){
				vp.set(out);
			} else {
				ok = false;
			}
		} else { // return undefined if no json but healthy response
			vp.setUndefined();
		}
	} else {
		ok = false;
		JS_ReportError(c->cx, "%s", result);
	}
	return ok;
}

bool wrapGoSetter(JSContext *cx,  JS::Handle<JSObject*> obj, JS::Handle<jsid> propid, bool x, JS::MutableHandle<JS::Value> vp) {
	JSAPIContext *c = (JSAPIContext*)JS_GetContextPrivate(cx);
	JSAutoRequest ar(cx);
	JSAutoCompartment ac(cx, c->o);
	// get name
    RootedValue idvalue(cx, IdToValue(propid));
    RootedString idstring(cx, ToString(cx, idvalue));
    JSAutoByteString idstr;
    if (!idstr.encodeLatin1(cx, idstring)){
		JS_ReportError(c->cx, "%s", "property id was not a valid string");
        return false;
	}
	// convert to json 
	jsonBuffer buf;
	buf.str = NULL;
	buf.cx = c->cx;
	buf.o = c->o;
	buf.n = 0;
	RootedObject replacer(c->cx);
	RootedValue undefined(c->cx);
	JS_Stringify(c->cx, vp, replacer, undefined, stringifier, &buf);
	// call go
	bool ok = true;
	char* result = NULL;
	RootedValue out(cx);
	if( go_setter(c, objId(c, obj), idstr.ptr(), buf.str, int(buf.n), &result) ){
		if( strlen(result) > 0 ){
			RootedString resultstr(c->cx, JS_NewStringCopyZ(c->cx, result));
			if( JS_ParseJSON(c->cx, resultstr, &out) ){
				vp.set(out);
			} else {
				ok = false;
			}
		} else { // return undefined if no json but healthy response
			vp.setUndefined();
		}
	} else {
		ok = false;
		JS_ReportError(c->cx, "%s", result);
	}
	free(buf.str);
	return ok;
}

jerr JSAPI_DefineProperty(JSAPIContext *c, uint32_t pid, char *name){
	JSAutoRequest ar(c->cx);
	JSAutoCompartment ac(c->cx, c->o);
	RootedObject p(c->cx, idToObj(c, pid));
	RootedValue undefined(c->cx, UndefinedValue());
	bool ok = JS_DefineProperty(
			c->cx,        // context
			p,            // prop's owner
			name,         // prop name
			undefined,    // initial value
			JSPROP_ENUMERATE | JSPROP_SHARED,
			wrapGoGetter, // getter callback
			wrapGoSetter // setter callback
			);
	if( !ok ){
		return JSAPI_FAIL;
	}
	return JSAPI_OK;
}

static JSRuntime *grt = NULL;
static JSContext *gcx = NULL;

// Inits the js runtime
// The docs say we need to create the first rt and cx on
// the main thread... seems a bit weird to just have these
// lying around but hey-ho
jerr JSAPI_Init() {
    if (!JS_Init()){
		return JSAPI_FAIL;
	}
    // Create global runtime
    grt = JS_NewRuntime(1L * 1024L * 1024L * 1024L, 0);
    if (!grt) {
		return JSAPI_FAIL;
	}
	// Create global context
	gcx = JS_NewContext(grt, 8192);
	if (!gcx) {
		return JSAPI_FAIL;
	}
	return JSAPI_OK;
}

jerr JSAPI_ThreadCanAccessRuntime() {
    if( !CurrentThreadCanAccessRuntime(grt) ){
		return JSAPI_FAIL;
	}
	return JSAPI_OK;
}

jerr JSAPI_ThreadCanAccessContext(JSAPIContext* c) {
    if( !CurrentThreadCanAccessRuntime(c->rt) ){
		return JSAPI_FAIL;
	}
	return JSAPI_OK;
}


jerr JSAPI_DestroyContext(JSAPIContext *c){
	if( c != NULL ){
		JS_DestroyContext(c->cx);
		free(c);
	}
	return JSAPI_OK;
}

void JSAPI_FreeChar(JSAPIContext *c, char *p){
	JS_free(c->cx, p);
}

// Executes javascript source string and returns response as
// JSON string (outstr).
// Returns JSAPI_OK on success.
// NOTE: outstr requires freeing on success.
jerr JSAPI_EvalJSON(JSAPIContext *c, char *source, char *filename, char **outstr, int *outlen){
    JSAutoRequest ar(c->cx);
    JSAutoCompartment ac(c->cx, c->o);
    RootedObject global(c->cx, c->o);
	RootedValue rval(c->cx);
	// eval
	if (!JS_EvaluateScript(c->cx, global, source, strlen(source), filename, 1, &rval)) {
		return JSAPI_FAIL;
	}
	// convert to json 
	jsonBuffer buf;
	buf.str = NULL;
	buf.cx = c->cx;
	buf.o = c->o;
	buf.n = 0;
	RootedObject replacer(c->cx);
	RootedValue undefined(c->cx);
	if( !JS_Stringify(c->cx, &rval, replacer, undefined, stringifier, &buf) ){
		if( buf.str != NULL ){
			free(buf.str);
		}
		return JSAPI_FAIL;
	}
	*outstr = buf.str;
	*outlen = buf.n;
	return JSAPI_OK;
}

// Executes javascript source string and discards any response.
jerr JSAPI_Eval(JSAPIContext *c, char *source, char *filename){
    JSAutoRequest ar(c->cx);
    JSAutoCompartment ac(c->cx, c->o);
    RootedObject global(c->cx, c->o);
	RootedValue rval(c->cx);
	// eval
	if (!JS_EvaluateScript(c->cx, global, source, strlen(source), filename, 1, &rval)) {
		return JSAPI_FAIL;
	}
	return JSAPI_OK;
}


///////////////////////////////// WORKER


struct WorkerInput {
    JSRuntime* runtime;
	int id;

    WorkerInput(JSRuntime* runtime, int id)
      : runtime(runtime), id(id)
    {}

    ~WorkerInput() {
    }
};

static void ContextWorker(void *arg){
	bool ok = false;
	JSAPIContext c;
	WorkerInput *input = (WorkerInput *) arg;
	c.id = input->id;
	do {
		// new rt with global parent runtime
		c.rt = JS_NewRuntime(1L * 1024L * 1024L * 1024L, 2L * 1024L * 1024L, input->runtime);
		if (!c.rt) {
			go_worker_fail(c.id, "failed to make global");
			break;
		}
		// new context
		c.cx = JS_NewContext(c.rt, 8192);
		if (!c.cx) {
			go_worker_fail(c.id, "failed to make global");
			break;
		}
		JSAutoRequest ar(c.cx);
		// error handlers
		JS_SetErrorReporter(c.cx, reportError);
		JS::SetOutOfMemoryCallback(c.rt, reportOOM, &c);
		// Create the global object
		c.o = JS_NewGlobalObject(c.cx, &global_class, nullptr, JS::DontFireOnNewGlobalHook);
		JSAutoCompartment ac(c.cx, c.o);
		RootedObject global(c.cx, c.o);
		if (!global) {
			go_worker_fail(c.id, "failed to make global");
			break;
		}
		if (!JS_InitStandardClasses(c.cx, global)) {
			go_worker_fail(c.id, "failed to init global classes");
			break;
		}
		js::SetDefaultObjectForContext(c.cx, global);
		JS_FireOnNewGlobalObject(c.cx, global);
		JS_SetContextPrivate(c.cx, &c);
		// Add objs store
		c.objs = JS_NewArrayObject(c.cx, 0);
		RootedValue objsv(c.cx, OBJECT_TO_JSVAL(c.objs));
		if (!JS_SetProperty(c.cx, global, "__objdefs__", objsv) ){
			go_worker_fail(c.id, "failed to create objdefs store");
			break;
		}
		RootedValue globalv(c.cx, OBJECT_TO_JSVAL(c.o));
		RootedObject objs(c.cx, c.objs);
		if (!JS_SetElement(c.cx, objs, 0, globalv) ){
			go_worker_fail(c.id, "failed to assign global to the objdefs store");
			break;
		}
		ok = true;
	} while(0);
	// worker thread
	if( ok ){
	    go_worker_wait(input->id, &c);
	}
	// Free
	if( c.cx ){
	    JS_DestroyContext(c.cx);
	}
	if( c.rt ){
	    JS_DestroyRuntime(c.rt);
	}
    js_delete(input);
	return;
}

Vector<PRThread *, 0, SystemAllocPolicy> workerThreads;

jerr JSAPI_NewContext(int id){

    WorkerInput *input = js_new<WorkerInput>(grt, id);
    if (!input) {
        return JSAPI_FAIL;
	}

    PRThread *thread = PR_CreateThread(PR_USER_THREAD, ContextWorker, input,
                                       PR_PRIORITY_NORMAL, PR_GLOBAL_THREAD, PR_JOINABLE_THREAD, 0);
    if (!thread || !workerThreads.append(thread)) {
        return JSAPI_FAIL;
	}

    return JSAPI_OK;
}
