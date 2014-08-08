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

struct JSAPIContext {
	JSRuntime *rt;
	JSContext *cx;
	JSObject *o;
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


/* The class of the global object. */
static const JSClass global_class = {
    "global", JSCLASS_NEW_RESOLVE | JSCLASS_GLOBAL_FLAGS,
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

JSObject* JSAPI_DefineObject(JSAPIContext *c, JSObject* parent, char *name){
	if( parent == NULL ){
		parent = c->o;
	}
	JSAutoRequest ar(c->cx);
	JSAutoCompartment ac(c->cx, c->o);
	RootedObject p(c->cx, parent);
	JSObject* obj = JS_DefineObject(c->cx, p, name, nullptr, JS::NullPtr(), 0);
	return obj;
}

struct jsonBuffer {
	JSAPIContext *c;
	char *str;
	uint32_t n;
};

bool stringifier(const jschar *s, uint32_t n, void *data){
	jsonBuffer *buf = (jsonBuffer*)data;
    JSAutoRequest ar(buf->c->cx);
	JSAutoCompartment ac(buf->c->cx, buf->c->o);
	RootedString ss(buf->c->cx,JS_NewUCStringCopyN(buf->c->cx, s, n));
	size_t sn = JS_GetStringEncodingLength(buf->c->cx, ss);
	buf->str = (char*)realloc(buf->str, (buf->n + sn) * sizeof(char));
	if( buf->str == NULL){
		printf("could not realloc during stringify");
		return false;
	}
	buf->n = JS_EncodeStringToBuffer(buf->c->cx, ss, buf->str+buf->n, sn) + buf->n;
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
	buf.c = c;
	buf.n = 0;
	RootedObject replacer(c->cx);
	RootedValue undefined(c->cx);
	JS_Stringify(c->cx, &argValues, replacer, undefined, stringifier, &buf);
	// send to go and parse resulting json
	bool ok = true;
	char *result = NULL;
	RootedValue out(cx);
	if( go_callback(c, JS_THIS_OBJECT(c->cx, vp), name, buf.str, int(buf.n), &result) ){
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

jerr JSAPI_DefineFunction(JSAPIContext *c, JSObject* parent, char *name){
	if( parent == NULL ){
		parent = c->o;
	}
	JSAutoRequest ar(c->cx);
	JSAutoCompartment ac(c->cx, c->o);
	RootedObject p(c->cx, (JSObject*) parent);
	JS_DefineFunction(c->cx, p, name, wrapGoFunction, 0, 0);
	return JSAPI_OK;
}

bool wrapGoGetter(JSContext *cx,  JS::HandleObject obj, JS::Handle<jsid> propid, JS::MutableHandle<JS::Value> vp) {
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
	// call go
	bool ok = true;
	char* result = NULL;
	RootedValue out(cx);
	if( go_getter(c, obj, idstr.ptr(), &result) ){
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
	buf.c = c;
	buf.n = 0;
	RootedObject replacer(c->cx);
	RootedValue undefined(c->cx);
	JS_Stringify(c->cx, vp, replacer, undefined, stringifier, &buf);
	// call go
	bool ok = true;
	char* result = NULL;
	RootedValue out(cx);
	if( go_setter(c, obj, idstr.ptr(), buf.str, int(buf.n), &result) ){
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

jerr JSAPI_DefineProperty(JSAPIContext *c, JSObject* parent, char *name){
	if( parent == NULL ){
		parent = c->o;
	}
	JSAutoRequest ar(c->cx);
	JSAutoCompartment ac(c->cx, c->o);
	RootedObject p(c->cx, parent);
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

// Inits the js runtime and returns the thread id it's running on
jerr JSAPI_Init() {
    if (!JS_Init()){
		return JSAPI_FAIL;
	}
    // Create global runtime
    grt = JS_NewRuntime(2048L * 1024L * 1024L, 0);
    if (!grt) {
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


JSAPIContext* JSAPI_NewContext(){
	JSAPIContext *c = (JSAPIContext*)malloc(sizeof(JSAPIContext));
	// use global runtime
	c->rt = grt;
	// Create a new context
	c->cx = JS_NewContext(c->rt, 8192);
	if (!c->cx) {
		printf("failed to make cx\n");
		return NULL;
	}
	// Start request
	JSAutoRequest ar(c->cx);
	/* Erros */
	JS_SetErrorReporter(c->cx, reportError);
	JS::SetOutOfMemoryCallback(c->rt, reportOOM, c);
	// Create the global object
	c->o = JS_NewGlobalObject(c->cx, &global_class, nullptr, JS::DontFireOnNewGlobalHook);
	JSAutoCompartment ac(c->cx, c->o);
	RootedObject global(c->cx, c->o);
	if (!global) {
		printf("failed to make global");
		return NULL;
	}
	if (!JS_InitStandardClasses(c->cx, global)) {
		printf("failed to init global classes\n");
		return NULL;
	}
	c->o = global;
	JS_FireOnNewGlobalObject(c->cx, global);
	// store context
	JS_SetContextPrivate(c->cx, c);
	return c;
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
	if (!JS_EvaluateScript(c->cx, global, source, strlen(source), filename, 0, &rval)) {
		return JSAPI_FAIL;
	}
	// convert to json 
	jsonBuffer buf;
	buf.str = NULL;
	buf.c = c;
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
	if (!JS_EvaluateScript(c->cx, global, source, strlen(source), filename, 0, &rval)) {
		return JSAPI_FAIL;
	}
	return JSAPI_OK;
}



