#include "lib/js.hpp"
#include "_cgo_export.h"

void Init(){
	go_callback = callFunction;
	go_error = reporter;
	go_getter = getprop;
	go_setter = setprop;
	go_worker_wait = workerWait;
	go_worker_fail = workerFail;
}

