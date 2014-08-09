package jsapi

import "fmt"

func ExampleContext_DefineObject_Empty() {
	cx := NewContext()
	cx.DefineObject("o", nil) // equivilent to `o = {}` in js
}

func ExampleContext_DefineObject_Proxy() {
	// Create a simple Person type
	type Person struct {
		Name string
	}
	p := &Person{"jeff"}

	// Create a context and map our person into it
	cx := NewContext()
	cx.DefineObject("person", p)

	// Read the name from javascript
	var name string
	cx.Eval(`person.Name`, &name)
	fmt.Println(name)

	// Set the name from javascript
	cx.Exec(`person.Name = 'bob'`)
	fmt.Println(p.Name)
	// Output:
	// jeff
	// bob
}
