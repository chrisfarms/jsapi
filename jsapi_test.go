package jsapi

import(
	"testing"
	"time"
	"fmt"
	"runtime"
	"sync"
)

func TestEvalNumber(t *testing.T) {

	cx := NewContext()
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

func TestEvalString(t *testing.T) {

	cx := NewContext()
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

func TestEvalDate(t *testing.T) {

	cx := NewContext()
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

func TestEvalErrors(t *testing.T) {

	cx := NewContext()
	defer cx.Destroy()

	err := cx.Exec(`throw new Error('ERROR1');`)
	if err == nil {
		t.Fatalf("expected an error to be returned")
	}
	r,ok := err.(*ErrorReport)
	if !ok {
		t.Fatalf("expected the error to be an ErrorReport but got: %T %v", err, err)
	}
	if r.Message != "Error: ERROR1" {
		t.Fatalf(`expected error message to be "Error: ERROR1" but got %q`, r.Message)
	}
}

func TestObjectWithIntFunction(t *testing.T) {

	cx := NewContext()
	defer cx.Destroy()

	math := cx.DefineObject("math")

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

func TestObjectWithVaridicFunction(t *testing.T) {

	cx := NewContext()
	defer cx.Destroy()

	obj := cx.DefineObject("fmt")

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

func TestGlobalVaridicFunction(t *testing.T) {

	cx := NewContext()
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

func TestSleepContext(t *testing.T) {

	cx := NewContext()
	defer cx.Destroy()


	cx.DefineFunction("sleep", func(ms int) {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	})

	err := cx.Exec(`sleep(10)`)
	if err != nil {
		t.Fatal(err)
	}

}

func TestErrorsInFunction(t *testing.T) {

	cx := NewContext()
	defer cx.Destroy()

	obj := cx.DefineObject("errs")

	fn := obj.DefineFunction("raise", func(msg string) {
		panic(msg)
	})

	if fn.Name != "raise" {
		t.Fatalf("expected func object to have name")
	}

	err := cx.Exec(`errs.raise('BANG')`)
	if err == nil {
		t.Fatalf("expected an error to be returned")
	}
	r,ok := err.(*ErrorReport)
	if !ok {
		t.Fatalf("expected the error to be an ErrorReport but got: %T %v", err, err)
	}
	exp := fmt.Sprintf("Error: %s: BANG", fn.Name)
	if r.Message != exp {
		t.Fatalf(`expected error message to be %q but got %q`, exp, r.Message)
	}

}

func TestOneContextManyGoroutines(t *testing.T) {

	runtime.GOMAXPROCS(20)

	cx := NewContext()
	defer cx.Destroy()

	cx.DefineFunction("snooze", func(ms int) bool {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return true
	})

    wg := new(sync.WaitGroup)
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            for j := 0; j < 100; j++ {
				var ok bool
				err := cx.Eval(`snooze(1)`, &ok)
				if err != nil {
					panic(err)
				}
				if !ok {
					panic("expected ok response")
				}
            }
            wg.Done()
        }()
    }
    wg.Wait()

}


