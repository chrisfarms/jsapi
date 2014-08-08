#include "lib/js.hpp"
#include "_cgo_export.h"

void Init(){
	go_callback = callback;
	go_error = reporter;
	go_getter = getprop;
	go_setter = setprop;
}

