#!/usr/bin/env bash

set -e
set -x

LIB=lib
MOZJS=moz/js/src
MOZBUILD=build-release
MOZDIST=$MOZJS/$MOZBUILD/dist

if [[ ! -e "${LIB}/libjs.a" ]]; then (
	cd $LIB/$MOZJS && 
	rm -rf $MOZBUILD &&
	autoconf && 
	mkdir -p $MOZBUILD && 
	cd $MOZBUILD &&
	../configure --disable-shared-js --enable-nspr-build --enable-debug &&
	make &&
	mv $(readlink -f dist/lib/libjs_static.a) ../../../../libjs.a
) fi

if [[ ! -e "${LIB}/libjsapi.a" ]]; then (
	cd $LIB &&
	rm -f libjsapi.a &&
	g++ -fPIC -c -std=c++11 -pthread -pipe -Wno-write-strings \
		-Wno-invalid-offsetof \
		-include $MOZDIST/include/js/RequiredDefines.h \
		-pthread \
		-include $MOZJS/$MOZBUILD/js/src/js-confdefs.h \
		-I$MOZDIST/include/ \
		-I$MOZJS \
		-I$MOZDIST/include/nspr \
		-o jsapi.o js.cpp &&
	ar crv libjsapi.a jsapi.o
) fi

go test -short


#c++ -o Unified_cpp_js_src_shell0.o -c  
#-I../../../dist/system_wrappers 
#-include /home/chrisfarms/src/github.com/chrisfarms/jsapi/lib/moz/config/gcc_hidden.h 
#-DEXPORT_JS_API -DIMPL_MFBT -DNO_NSPR_10_SUPPORT 
#-I/home/chrisfarms/src/github.com/chrisfarms/jsapi/lib/moz/js/src/shell 
#-I. -I/home/chrisfarms/src/github.com/chrisfarms/jsapi/lib/moz/js/src/shell/.. 
#-I.. -I../../../dist/include  
#-I/home/chrisfarms/src/github.com/chrisfarms/jsapi/lib/moz/js/src/build-release/dist/include/nspr        
#-fPIC   -DMOZILLA_CLIENT 
#-include ../../../js/src/js-confdefs.h -MD -MP -MF 
#.deps/Unified_cpp_js_src_shell0.o.pp  -Wall -Wpointer-arith -Woverloaded-virtual 
#-Werror=return-type -Werror=int-to-pointer-cast -Wtype-limits 
#-Wempty-body -Werror=conversion-null -Wsign-compare -Wno-invalid-offsetof 
#-Wcast-align -fno-rtti -fno-exceptions -fno-math-errno 
#-std=gnu++0x -pthread -pipe  -DDEBUG -DTRACING -g -O3 -freorder-blocks  
#-fno-omit-frame-pointer      
#Unified_cpp_js_src_shell0.cp

# /home/chrisfarms/src/github.com/chrisfarms/jsapi/lib/moz/js/src/build-release/_virtualenv/bin/python /home/chrisfarms/src/github.com/chrisfarms/jsapi/lib/moz/config/expandlibs_exec.py --uselist --  c++ -o js  -Wall -Wpointer-arith -Woverloaded-virtual -Werror=return-type -Werror=int-to-pointer-cast -Wtype-limits -Wempty-body -Werror=conversion-null -Wsign-compare -Wno-invalid-offsetof -Wcast-align -fno-rtti -fno-exceptions -fno-math-errno -std=gnu++0x -pthread -pipe  -DDEBUG -DTRACING -g -O3 -freorder-blocks  -fno-omit-frame-pointer  Unified_cpp_js_src_shell0.o   -lpthread  -Wl,-z,noexecstack -Wl,-z,text -Wl,--build-id    -Wl,-rpath-link,../../../dist/bin -Wl,-rpath-link,/usr/local/lib    ../../../js/src/editline/libeditline.a ../../../js/src/libjs_static.a  -L/home/chrisfarms/src/github.com/chrisfarms/jsapi/lib/moz/js/src/build-release/dist/lib -lnspr4 -lplc4 -lplds4 -lm -ldl  -lm -ldl 
