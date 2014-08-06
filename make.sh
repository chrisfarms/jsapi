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
	make &&
	mv $(readlink -f dist/lib/libjs_static.a) ../../../../
) && (
	cd $LIB &&
	rm -f libmonk.a &&
	g++ -fPIC -c -std=c++11 -Wno-write-strings \
		-Wno-invalid-offsetof \
		-include $MOZDIST/include/js/RequiredDefines.h \
		-I$MOZDIST/include/ \
		-I$MOZJS \
		-o monk.o js.cpp &&
	ar crv libmonk.a monk.o 
) && (
	go test && 
	go install
)
