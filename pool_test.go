package jsapi

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

const (
	POOL_SIZE = 8
	delay     = 10
	script    = `1+1`
)

func BenchmarkEvalPool(b *testing.B) {
	cx := NewPool(POOL_SIZE)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var result interface{}
			err := cx.Eval(script, &result)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestPoolInterface(t *testing.T) {
	var _ Evaluator = &Pool{}
	var _ Definer = &Pool{}
	var _ Definer = &ObjectPool{}
}

func TestPoolEvalNumber(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	var i int
	err := cx.Eval(`1+1`, &i)
	if err != nil {
		t.Fatal(err)
	}

	if i != 2 {
		t.Fatalf("expected 1+1 to eval to 2 but got %d", i)
	}

}

func TestPoolEvalString(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	var s string
	err := cx.Eval(`"h"+"ello"`, &s)
	if err != nil {
		t.Fatal(err)
	}

	if s != "hello" {
		t.Fatalf("expected to eval to the string \"hello\" got %s", s)
	}

}

func TestPoolEvalDate(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	var v time.Time
	err := cx.Eval(`new Date('2012-01-01')`, &v)
	if err != nil {
		t.Fatal(err)
	}
	layout := "2006-01-02"
	if v.Format(layout) != "2012-01-01" {
		t.Fatalf("expected to eval to Date(2012-01-01) to time.Time got %s", v.Format(layout))
	}

}

func TestPoolEvalErrors(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	err := cx.Exec(`throw new Error('ERROR1');`)
	if err == nil {
		t.Fatalf("expected an error to be returned")
	}
	r, ok := err.(*ErrorReport)
	if !ok {
		t.Fatalf("expected the error to be an ErrorReport but got: %T %v", err, err)
	}
	if r.Message != "Error: ERROR1" {
		t.Fatalf(`expected error message to be "Error: ERROR1" but got %q`, r.Message)
	}
}

func TestPoolObjectWithIntFunction(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	math, _ := cx.DefineObject("math", nil)

	math.DefineFunction("add", func(a int, b int) int {
		return a + b
	})

	var i int
	err := cx.Eval(`math.add(1,2)`, &i)
	if err != nil {
		t.Fatal(err)
	}
	if i != 3 {
		t.Fatalf("expected math.add(1,2) to return 3 but got %d", i)
	}

}

func TestPoolNestedObjects(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	parent, _ := cx.DefineObject("parent", nil)
	child, _ := parent.DefineObject("child", nil)

	child.DefineFunction("greet", func() string {
		return "hello"
	})

	var s string
	err := cx.Eval(`parent.child.greet()`, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s != "hello" {
		t.Fatalf(`expected parent.child.greet() to return "hello" but got %s`, s)
	}

}

func TestPoolObjectWithVaridicFunction(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	obj, _ := cx.DefineObject("fmt", nil)

	obj.DefineFunction("sprintf", func(format string, args ...interface{}) string {
		return fmt.Sprintf(format, args...)
	})

	var s string
	err := cx.Eval(`fmt.sprintf('%.0f/%.0f/%s', 1, 2.0, "3")`, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s != "1/2/3" {
		t.Fatalf(`expected to return "1/2/3" but got %q`, s)
	}

}

func TestPoolGlobalVaridicFunction(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	cx.DefineFunction("sprintf", func(format string, args ...interface{}) string {
		return fmt.Sprintf(format, args...)
	})

	var s string
	err := cx.Eval(`sprintf('%.0f/%.0f/%s', 1, 2.0, "3")`, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s != "1/2/3" {
		t.Fatalf(`expected to return "1/2/3" but got %q`, s)
	}

}

func TestPoolSleepContext(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	cx.DefineFunction("sleep", func(ms int) {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	})

	err := cx.Exec(`sleep(1)`)
	if err != nil {
		t.Fatal(err)
	}

}

func TestPoolErrorsInFunction(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	obj, _ := cx.DefineObject("errs", nil)

	obj.DefineFunction("raise", func(msg string) {
		panic(msg)
	})

	err := cx.Exec(`errs.raise('BANG')`)
	if err == nil {
		t.Fatalf("expected an error to be returned")
	}
	r, ok := err.(*ErrorReport)
	if !ok {
		t.Fatalf("expected the error to be an ErrorReport but got: %T %v", err, err)
	}
	exp := fmt.Sprintf("Error: raise: BANG")
	if r.Message != exp {
		t.Fatalf(`expected error message to be %q but got %q`, exp, r.Message)
	}

}

func TestPoolObjectProperties(t *testing.T) {

	type Person struct {
		Name string
		Age  int
	}

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	person := &Person{"jeff", 22}

	cx.DefineObject("o", person)

	var s string
	err := cx.Eval(`o.name`, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s != person.Name {
		t.Fatalf(`expected to get value of person.Name (%q) from js but got %q`, person.Name, s)
	}

	err = cx.Exec(`o.name = "geoff"`)
	if err != nil {
		t.Fatal(err)
	}
	if person.Name != "geoff" {
		t.Fatalf(`expected to set value of person.Name to "geoff" but got %q`, person.Name)
	}

	var i int
	err = cx.Eval(`o.age`, &i)
	if err != nil {
		t.Fatal(err)
	}
	if i != person.Age {
		t.Fatalf(`expected to get value of person.Age (%d) from js but got %v`, person.Age, i)
	}

	err = cx.Exec(`o.age = 25`)
	if err != nil {
		t.Fatal(err)
	}
	if person.Age != 25 {
		t.Fatalf(`expected to set value of person.Age to 25 but got %v`, person.Age)
	}

}

func TestPoolOneContextManyGoroutines(t *testing.T) {

	if testing.Short() {
		t.Skip()
	}

	runtime.GOMAXPROCS(20)

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	cx.DefineFunction("snooze", func(ms int) bool {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return true
	})

	wg := new(sync.WaitGroup)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				var ok bool
				err := cx.Eval(`snooze(0)`, &ok)
				if err != nil {
					t.Error(err)
					return
				}
				if !ok {
					t.Errorf("expected ok response")
					return
				}
			}
		}()
	}
	wg.Wait()

}

func TestPoolManyContextManyGoroutines(t *testing.T) {

	if testing.Short() {
		t.Skip()
	}

	runtime.GOMAXPROCS(20)

	wg := new(sync.WaitGroup)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cx := NewPool(POOL_SIZE)
			defer cx.Destroy()

			cx.DefineFunction("snooze", func(ms int) bool {
				time.Sleep(time.Duration(ms) * time.Millisecond)
				return true
			})
			for j := 0; j < 50; j++ {
				var ok bool
				err := cx.Eval(`snooze(0)`, &ok)
				if err != nil {
					t.Error(err)
					return
				}
				if !ok {
					t.Errorf("expected ok response")
					return
				}
			}
		}()
	}
	wg.Wait()

}

func TestPoolExecFile(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()
	if err := cx.ExecFileAll("./jsapi_test.js"); err != nil {
		t.Fatal(err)
	}

	var ok bool
	if err := cx.Eval(`test()`, &ok); err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected test() function from jsapi_test.js file to return true got false")
	}

}

func TestPoolDeadlockCondition(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()
	cx.DefineFunction("mkfun", func() {
		cx.DefineFunction("dynamic", func() bool {
			return true
		})
	})
	if err := cx.Exec(`mkfun()`); err != nil {
		t.Fatal(err)
	}
	var ok bool
	if err := cx.Eval(`dynamic()`, &ok); err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal()
	}

}

type person struct {
	Name string
}

func (p *person) add(a, b int) int {
	return a + b
}

func TestPoolFunctionWithWeirdScope(t *testing.T) {

	cx := NewPool(POOL_SIZE)
	defer cx.Destroy()

	p := &person{"bob"}
	math, _ := cx.DefineObject("math", p)

	math.DefineFunction("add", p.add)

	var i int
	err := cx.Eval(`math.add2 = function(){ return math.add.apply(math, [1,2]) }; math.add2()`, &i)
	if err != nil {
		t.Fatal(err)
	}
	if i != 3 {
		t.Fatalf("expected math.add(1,2) (on proxy onject) to return 3 but got %d", i)
	}

}
