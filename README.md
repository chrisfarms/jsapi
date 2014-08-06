# JSAPI

JSAPI is a Go ([golang](http://golang.org)) package for embedding the spidermonkey javascript interpreter into your Go programs.

## Example

```go
	package main

	import( 
		"fmt"
		"github.com/chrisfarms/jsapi"
	)

	func main() {

		cx := jsapi.NewContext()

		cx.DefineFunction("add", func(a, b int) int {
			return a + b
		})
		
		var result int
		cx.Eval(`add(1,2)`, &result); err != nil {

		fmt.Println("result is", result)
	}
```

## Installation

### Prerequisites

* Go 1.3
* gcc

### If you are running an x86_64 architecture

If you are running on x86_64 architecture then you should be able to take advantage of the bundled binaries and get away with installing the `jsapi` package just as you would any other Go package by adding the import path `github.com/chrisfarms/jsapi` to your project and using `go get`

```sh
	go get github.com/chrisfarms/jsapi
```

## For everyone else

Since this package relies on linking 





LD_LIBRARY_PATH=/home/chrisfarms/src/github.com/chrisfarms/monkey/mozilla-central/js/src/build-release/dist/lib; (cd lib && g++ -fPIC -c -std=c++11 -Wno-write-strings -Wno-invalid-offsetof -include /home/chrisfarms/src/github.com/chrisfarms/monkey/mozilla-central/js/src/build-release/dist/include/js/RequiredDefines.h -I/home/chrisfarms/src/github.com/chrisfarms/monkey/mozilla-central/js/src/build-release/dist/include/ -I/home/chrisfarms/src/github.com/chrisfarms/monkey/mozilla-central/js/src -o monk.o js.cpp && ar rvs libmonk.a monk.o) && go build && go test
