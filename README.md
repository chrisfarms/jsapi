# JSAPI

## Overview

JSAPI is a Go ([golang](http://golang.org)) package for embedding the spidermonkey javascript interpreter into Go projects.

## Quick Tour

#### Exposing a Go function to javascript

Exposing a simple Go function to javascript, calling the function and printing the resulting value out.

```go
cx := jsapi.NewContext()

cx.DefineFunction("add", func(a, b int) int {
	return a + b
})

var result int
cx.Eval(`add(1,2)`, &result) // call the go func from js

fmt.Println("result is", result)
```

#### Mapping a Go struct to a javascript Object

It is often useful to have simple struct properties visible from both Go-land and JS-land, by passing a struct to `DefineObject` the values will be proxied back and forth:

```go
type Person struct {
    Name string
}
p := &Person{"jeff"}

cx := jsapi.NewContext()

cx.DefineObject("person", p) // p's public fields exposed

var name string
cx.Eval(`person.name`, &name) // Read the name from js

cx.Exec(`person.name = 'bob'`) // Set the name from js
```

#### Use a Pool of worker contexts

Avoid bottlenecks in certain loads with Pools:

```go
// create pool of 8 worker Contexts
pool := jsapi.NewPool(8) 

// Add a sleep function to all the contexts
pool.DefineFunction("sleep", func(ms int){
	time.Sleep(time.Duration(ms) * time.Millisecond)	
})

// Spawn 100 goroutines to use the pool
wg := new(sync.WaitGroup)
for i := 0; i < 100; i++ {
	wg.Add(1)
	go func() {
		pool.Exec(`sleep(10)`)
		wg.Done()
	}()
}
wg.Wait()
```

## Documentation

See [godoc](http://godoc.org/github.com/chrisfarms/jsapi) for API documentation.

## Installation

Since this package relies on a C/C++ library that steps outside the realm of the `go` tool's capabilities you will have to perform some extra steps to get it to build.

First ensure that you have your `GOPATH` configured to something suitable. Then fetch and build jsapi manually using the following steps:

```sh
mkdir -p $GOPATH/src/github.com/chrisfarms/jsapi
cd $GOPATH/src/github.com/chrisfarms/jsapi
git clone --recursive https://github.com/chrisfarms/jsapi.git "."
./make.sh
go install
```

If all went well you should see the `PASS` output from the test run and the package can now be used as per usual via `go install` and:

```go
import "github.com/chrisfarms/jsapi"
```

## 






