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
	../configure --disable-shared-js --enable-debug &&
	make &&
	mv $(readlink -f dist/lib/libjs_static.a) ../../../../libjs.a
) fi

if [[ ! -e "${LIB}/libjsapi.a" ]]; then (
	cd $LIB &&
	rm -f libjsapi.a &&
	g++ -fPIC -c -std=c++11 -Wno-write-strings \
		-Wno-invalid-offsetof \
		-include $MOZDIST/include/js/RequiredDefines.h \
		-I$MOZDIST/include/ \
		-I$MOZJS \
		-o jsapi.o js.cpp &&
	ar crv libjsapi.a jsapi.o
) fi

go test
