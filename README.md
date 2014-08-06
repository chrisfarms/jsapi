# JSAPI

## Overview

JSAPI is a Go ([golang](http://golang.org)) package for embedding the spidermonkey javascript interpreter into your Go projects.

## Example

Here is a quick example of exposing a simple Go function to javascript, calling the function and printing the resulting value out.

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
	cx.Eval(`add(1,2)`, &result)

	fmt.Println("result is", result)
}
```

## Documentation

See godoc for API documentation.

## Installation

Since this package relies on a C/C++ library that steps outside the realm of the `go` tool's capabilities you will have to perform some extra steps to get it to build.

First ensure that you have your `GOPATH` configured to something suitable. Then fetch and build jsapi manually using the following steps:

```sh
mkdir -p $GOPATH/src/github.com/chrisfarms/jsapi
cd $GOPATH/src/github.com/chrisfarms/jsapi
git clone --recursive https://github.com/chrisfarms/jsapi.git "."
./make.sh
```

If all went well the package should now be installed in your `GOPATH` ready to be imported in your project via:

```go
import "github.com/chrisfarms/jsapi"
```








