
#ifndef jsapiwrap_h
#define jsapiwrap_h

#ifdef __cplusplus
extern "C" {
#endif

#ifndef __cplusplus
/* this is just for cgo */
#include <inttypes.h>
typedef void* JSObject;
typedef struct {
	void* rt;
	void* cx;
	JSObject* o;
} JSAPIContext;
#endif

typedef int jerr;

typedef int (*GoFun)(JSAPIContext* c, JSObject* o, char* name, char* s, int len, char** result);
typedef void (*GoErr)(JSAPIContext* c, char* filename, unsigned int line, char* msg);
typedef int (*GoGet)(JSAPIContext* c, JSObject* o, char* name, char** result);
typedef int (*GoSet)(JSAPIContext* c, JSObject* o, char* name, char* s, int len, char** result);
typedef void (*GoWork)(char* name, char* s, int len, char* errMsg);

#define JSAPI_OK 0
#define JSAPI_FAIL 1

GoFun go_callback;
GoErr go_error;
GoGet go_getter;
GoSet go_setter;
GoWork go_worker_callback;

JSAPIContext* JSAPI_NewContext();

jerr JSAPI_Init();
jerr JSAPI_ThreadCanAccessRuntime();
jerr JSAPI_DestroyContext(JSAPIContext* c);
jerr JSAPI_EvalJSON(JSAPIContext* c, char* source, char* filename, char** outstr, int* outlen);
jerr JSAPI_EvalJSONWorker(JSAPIContext *c, char *source, char *name);
jerr JSAPI_Eval(JSAPIContext* c, char* source, char* filename);
void JSAPI_FreeChar(JSAPIContext* c, char* p);
jerr JSAPI_DefineFunction(JSAPIContext* c, JSObject* parent, char* name);
jerr JSAPI_DefineProperty(JSAPIContext* c, JSObject* parent, char* name);
JSObject* JSAPI_DefineObject(JSAPIContext* c, JSObject* parent, char* name);

#ifdef __cplusplus
}
#endif

#endif
