
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
	JSObject* objs;
	int id;
} JSAPIContext;
#endif

typedef int jerr;

typedef int (*GoFun)(JSAPIContext* c, uint32_t fid, char* name, char* s, int len, char** result);
typedef void (*GoErr)(JSAPIContext* c, char* filename, unsigned int line, char* msg);
typedef int (*GoGet)(JSAPIContext* c, uint32_t oid, char* name, char** result);
typedef int (*GoSet)(JSAPIContext* c, uint32_t oid, char* name, char* s, int len, char** result);
typedef void (*GoWorkWait)(int id, JSAPIContext* c);
typedef void (*GoWorkFail)(int id, char* err);

#define JSAPI_OK 0
#define JSAPI_FAIL 1

GoFun go_callback;
GoErr go_error;
GoGet go_getter;
GoSet go_setter;
GoWorkWait go_worker_wait;
GoWorkFail go_worker_fail;

jerr JSAPI_NewContext(int cid);
jerr JSAPI_Init();
jerr JSAPI_ThreadCanAccessRuntime();
jerr JSAPI_ThreadCanAccessContext(JSAPIContext* c);
jerr JSAPI_DestroyContext(JSAPIContext* c);
jerr JSAPI_EvalJSON(JSAPIContext* c, char* source, char* filename, char** outstr, int* outlen);
jerr JSAPI_Eval(JSAPIContext* c, char* source, char* filename);
void JSAPI_FreeChar(JSAPIContext* c, char* p);
jerr JSAPI_DefineFunction(JSAPIContext* c, uint32_t pid, char* name, uint32_t fid);
jerr JSAPI_DefineProperty(JSAPIContext* c, uint32_t pid, char* name);
jerr JSAPI_DefineObject(JSAPIContext* c, uint32_t pid, char* name, uint32_t oid);

#ifdef __cplusplus
}
#endif

#endif
