package jsapi_test

import (
	"fmt"
	"github.com/chrisfarms/jsapi"
)

func ExampleContext_DefineObject_Empty() {
	cx := jsapi.NewContext()
	cx.DefineObject("o", nil) // equivilent to `o = {}` in js
}

func ExampleContext_DefineObject_Proxy() {
	// Create a simple Person type
	type Person struct {
		Name string
	}
	p := &Person{"jeff"}

	// Create a context and map our person into it
	cx := jsapi.NewContext()
	cx.DefineObject("person", p)

	// Read the name from javascript
	var name string
	cx.Eval(`person.Name`, &name)
	fmt.Println("Read name from js as:", name)

	// Set the name from javascript
	cx.Exec(`person.Name = 'bob'`)
	fmt.Println("Set name from js to:", p.Name)
	// Output:
	// Read name from js as: jeff
	// Set name from js to: bob
}

func ExampleObject_DefineObject() {
	// Create a context with a global variable "o"
	// pointing to an empty object
	cx := jsapi.NewContext()
	o1, _ := cx.DefineObject("o", nil)

	// Add a "x" property to the object containing
	// another empty object
	o1.DefineObject("x", nil)

	// Do something with our objects in js land
	cx.Exec(`o.x.y = 1`)
}

func ExampleContext_DefineFunction_Simple() {
	// Create a context
	cx := jsapi.NewContext()

	// Add a really basic 'print' function to the context
	cx.DefineFunction("print", func(s string) {
		fmt.Println(s)
	})

	// try it out
	cx.Exec(`print('Hello from javascript-land')`)
	// Output:
	// Hello from javascript-land
}

func ExampleContext_DefineFunction_Add() {
	// Create a context
	cx := jsapi.NewContext()

	// Create an adder function
	cx.DefineFunction("add", func(a, b int) int {
		return a + b
	})

	// try it out
	var result int
	cx.Eval(`add(1,1)`, &result)
	fmt.Println("result:", result)
	// Output:
	// result: 2
}

func ExampleObject_DefineFunction_Vari() {
	// Create a context
	cx := jsapi.NewContext()

	// Here we will try to mimic sprintf from Go's fmt package
	// a bit by creating an object for our 'fmt'
	// namespace, then creating a variadic function in js-land
	// that returns a string
	ns, _ := cx.DefineObject("fmt", nil)
	ns.DefineFunction("sprintf", func(layout string, args ...interface{}) string {
		return fmt.Sprintf(layout, args...)
	})

	// try it out
	var result string
	cx.Eval(`fmt.sprintf('%.0f / %.1f / %.2f', 1,2,3)`, &result)
	fmt.Println("result:", result)
	// Output:
	// result: 1 / 2.0 / 3.00
}

func ExampleNewPool() {
	// Create a pool of 5 workers
	cx := jsapi.NewPool(5)

	// Use it just like a Context
	var result int
	cx.Eval(`1+1`, &result)

	fmt.Println("result:", result)
	// Output:
	// result: 2
}

func ExampleNewContext() {
	// Create a context
	cx := jsapi.NewContext()

	// Execute some javascript and fetch the result
	var result int
	cx.Eval(`1+1`, &result)

	fmt.Println("result:", result)
	// Output:
	// result: 2
}
