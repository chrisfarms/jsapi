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
    JS_ConvertStub,   nullptr,
    nullptr, nullptr, nullptr,
    JS_GlobalObjectTraceHook
};


// The error reporter callback.
void reportError(JSContext *cx, const char *message, JSErrorReport *report) {
	JSAPIContext *c = (JSAPIContext*)JS_GetContextPrivate(cx);
	go_error(c, report->filename, (unsigned int)report->lineno, message);
}

// The OOM reporter
void reportOOM(JSContext *cx, void *data) {
	JSAPIContext *c = (JSAPIContext*)data;
	fprintf(stderr, "spidermonkey has run out of memory!\n");
	go_error(c, "__fatal__", 0, "spidermonkey ran out of memory"); 
}



void* JSAPI_DefineObject(JSAPIContext *c, void *o, char *name){
	if( o == NULL ){
		o = c->o;
	}
    JSAutoRequest ar(c->cx);
	RootedObject parent(c->cx, (JSObject*)o);
    JSAutoCompartment ac(c->cx, parent);
	JSObject *obj = JS_DefineObject(c->cx, parent, name, nullptr, JS::NullPtr(), 0);
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
	RootedObject global(buf->c->cx, buf->c->o);
	JSAutoCompartment ac(buf->c->cx, global);
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
	//JS_AbortIfWrongThread(c->rt); //DEBUG
    JSAutoRequest ar(c->cx);
	RootedObject global(c->cx, c->o);
	JSAutoCompartment ac(c->cx, global);
	// get name
	JS::CallReceiver rec = JS::CallReceiverFromVp(vp);
	RootedObject callee(cx, &rec.callee());
	RootedValue nameval(cx);
	if( !JS_GetProperty(c->cx, callee, "name", &nameval) ){
		fprintf(stderr, "could not find callee name");
		return false;
	}
	RootedString namestr(c->cx, ToString(c->cx, nameval));
	char *name = JS_EncodeStringToUTF8(c->cx, namestr); 
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
	JSAPI_FreeChar(c, name);
	return ok;
}

void* JSAPI_DefineFunction(JSAPIContext *c, void *o, char *name){
	if( o == NULL ){
		o = c->o;
	}
    JSAutoRequest ar(c->cx);
	RootedObject parent(c->cx, (JSObject*) o);
	RootedObject global(c->cx, c->o);
	JSAutoCompartment ac(c->cx, global);
	JSFunction *fun = JS_DefineFunction(c->cx, parent, name, wrapGoFunction, 0, 0);
	return fun;
}

static JSRuntime *grt = NULL;
static JSContext *gcx = NULL;

void JSAPI_Init() {
    if (!JS_Init()){
		printf("failed to init\n");
	}
    // Create global runtime
    grt = JS_NewRuntime(2048L * 1024L * 1024L, 0);
    if (!grt) {
		printf("failed to make global runtime\n");
	}
}


JSAPIContext* JSAPI_NewContext(){
	JSAPIContext *c = (JSAPIContext*)malloc(sizeof(JSAPIContext));
    // use global runtime
    c->rt = grt;
    // Create a new context
    c->cx = JS_NewContext(c->rt, 8192);
    if (!c->cx) {
		printf("failed to make cx\n");
		return 0;
	}
	/* Erros */
    JS_SetErrorReporter(c->cx, reportError);
	JS::SetOutOfMemoryCallback(c->rt, reportOOM, c);
    /* Create the global object in a new compartment. */
    JSAutoRequest ar(c->cx);
    RootedObject global(c->cx, JS_NewGlobalObject(c->cx, &global_class, nullptr, JS::DontFireOnNewGlobalHook));
    JSAutoCompartment ac(c->cx, global);
	js::SetDefaultObjectForContext(c->cx, global);
	c->o = global;
    /* Populate the global object with the standard globals, like Object and Array. */
    if (!JS_InitStandardClasses(c->cx, global)) {
		printf("failed to init global classes\n");
        return 0;
	}
	/* assign our context */
	JS_SetContextPrivate(c->cx, c);
	return c;
}

int JSAPI_DestroyContext(JSAPIContext *c){
	if( c != NULL ){
		JS_DestroyContext(c->cx);
		free(c);
	}
	return 0;
}

void JSAPI_FreeChar(JSAPIContext *c, char *p){
	JS_free(c->cx, p);
}

// Executes javascript source string and returns response as
// JSON string (outstr).
// Returns JSAPI_OK on success.
// NOTE: outstr requires freeing on success.
int JSAPI_EvalJSON(JSAPIContext *c, char *source, char *filename, char **outstr, int *outlen){
    RootedObject global(c->cx, c->o);
    JSAutoRequest ar(c->cx);
    JSAutoCompartment ac(c->cx, global);
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
int JSAPI_Eval(JSAPIContext *c, char *source, char *filename){
    RootedObject global(c->cx, c->o);
    JSAutoRequest ar(c->cx);
    JSAutoCompartment ac(c->cx, global);
	RootedValue rval(c->cx);
	// eval
	if (!JS_EvaluateScript(c->cx, global, source, strlen(source), filename, 0, &rval)) {
		return JSAPI_FAIL;
	}
	return JSAPI_OK;
}



