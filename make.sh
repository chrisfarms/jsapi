#!/usr/bin/env bash

set -e
set -x

LIB=lib
MOZJS=moz/js/src
MOZBUILD=build-release
MOZDIST=$MOZJS/$MOZBUILD/dist

(
	cd $LIB/$MOZJS && 
	rm -rf $MOZBUILD &&
	autoconf && 
	mkdir -p $MOZBUILD && 
	cd $MOZBUILD &&
	../configure --disable-shared-js && 
	make
) && (
	cd $LIB && 
	rm -f libjs.a libjs.o libjs_static.a libmonk.a monk.o js.o &&
	g++ -fPIC -c -std=c++11 -Wno-write-strings \
		-Wno-invalid-offsetof \
		-include $MOZDIST/include/js/RequiredDefines.h \
		-I$MOZDIST/include/ \
		-I$MOZJS \
		-o monk.o js.cpp &&
	ar crv libmonk.a monk.o 
) && (
	go test
)
