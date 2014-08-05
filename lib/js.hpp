
#ifndef jsapiwrap_h
#define jsapiwrap_h

#ifdef __cplusplus
extern "C" {
#endif

#ifndef __cplusplus
/* this is just for cgo */
typedef struct {
	void *rt;
	void *cx;
	void *o;
} JSAPIContext;
#endif

typedef int (*GoFun)(JSAPIContext *c, void *o, char *name, char *s, int len, char **result);
typedef void (*GoErr)(JSAPIContext *c, const char *filename, unsigned int line, const char *msg);

#define JSAPI_OK 0
#define JSAPI_FAIL 1

GoFun go_callback;
GoErr go_error;

void JSAPI_Init();
JSAPIContext* JSAPI_NewContext();
int JSAPI_DestroyContext(JSAPIContext *c);
int JSAPI_EvalJSON(JSAPIContext *c, char *source, char *filename, char **outstr, int *outlen);
int JSAPI_Eval(JSAPIContext *c, char *source, char *filename);
void JSAPI_FreeChar(JSAPIContext *c, char *p);
void* JSAPI_DefineFunction(JSAPIContext *c, void *o, char *name);
void* JSAPI_DefineObject(JSAPIContext *c, void *o, char *name);

#ifdef __cplusplus
}
#endif

#endif
