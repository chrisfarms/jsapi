package jsapi

import (
	"io"
	"sync"
)

type pfn struct {
	call func(cx *Context)
	done chan bool
}

// Pool implements the Evaluator and Definer interfaces but
// is backed by a pool of contexts rather than just one.
// Since each context can only run one thread at a time certain
// workloads may find the context becomes a bottleneck and using
// Pool may give a significant performance boost if there are other
// cpu/cores to take advantage of.
//
// It is important to understand that Pool is made up of multiple
// totally seperate javascript Contexts. These Contexts cannot interact
// with each other. So for example if you called Exec to set a global
// variable it will have only been set in *one* of the contexts within
// the pool and there would be no guarentee that you would see it on the
// next call to Eval/Exec. For these reason there exists EvalAll and ExecAll
// functions on Pools, which allow for setting up the entire Pool.
//
// The general workflow for using Pool would be:
//
//     // Create a pool
//     p := NewPool(4)
//     // Setup the pool
//     p.ExecAll(`function MyAwesomeApp(){ .... }`)
//     // Use the workers without causing side effects to the global namespace
//     for i := 0; i<100; i++ {
//         go func(){
//             p.Exec(`MyAwecomeApp(1)`)
//         }()
//     }
type Pool struct {
	cxs   []*Context
	in    chan *pfn
	wg    sync.WaitGroup
	n     int  // readonly pool size
	Valid bool // is this pool active
}

// NewPool creates a pool of n worker contexts.
func NewPool(n int) *Pool {
	p := &Pool{}
	p.cxs = make([]*Context, n)
	p.in = make(chan *pfn)
	p.n = n
	p.Valid = true
	for i := 0; i < n; i++ {
		cx := NewContext()
		p.cxs[i] = cx
		p.wg.Add(1)
		go func(cx *Context) {
			for {
				select {
				case fn, ok := <-p.in:
					if !ok {
						cx.Destroy()
						p.wg.Done()
						return
					}
					fn.call(cx)
					fn.done <- true
				}
			}
		}(cx)
	}
	return p
}

// Create a go function mapping in ALL contexts within the pool.
// See context's description for more details.
func (p *Pool) DefineFunction(name string, fun interface{}) (err error) {
	for _, cx := range p.cxs {
		err = cx.DefineFunction(name, fun)
		if err != nil {
			return
		}
	}
	return nil
}

// Create an object in ALL contexts within the pool.
// See context's description for more details.
func (p *Pool) DefineObject(name string, proxy interface{}) (Definer, error) {
	op := &ObjectPool{p, make([]*Object, p.n)}
	for i, cx := range p.cxs {
		o, err := cx.DefineObject(name, proxy)
		if err != nil {
			return nil, err
		}
		op.objects[i] = o.(*Object)
	}
	return op, nil
}

// Execute source js in the first available worker context and return
// the result of the expression to result.
func (p *Pool) Eval(source string, result interface{}) (err error) {
	p.one(func(cx *Context) {
		err = cx.Eval(source, result)
	})
	return err
}

// Execute source js in the first available worker context.
// Errors are returned but the value of the expression is discarded.
func (p *Pool) Exec(source string) (err error) {
	p.one(func(cx *Context) {
		err = cx.Exec(source)
	})
	return err
}

// Execute js from a file in the next available worker context.
func (p *Pool) ExecFile(filename string) (err error) {
	p.one(func(cx *Context) {
		err = cx.ExecFile(filename)
	})
	return err
}

// Executes the given js source in ALL contexts.
// Execution will stop at the first error if one is raised.
func (p *Pool) ExecAll(source string) (err error) {
	for _, cx := range p.cxs {
		err = cx.Exec(source)
		if err != nil {
			break
		}
	}
	return err
}

// Load filename into ALL contexts in the pool.
// Execution will stop at the first error if one is raised.
func (p *Pool) ExecFileAll(filename string) (err error) {
	for _, cx := range p.cxs {
		err = cx.ExecFile(filename)
		if err != nil {
			break
		}
	}
	return err
}

// Execute js from an io.Reader in the first available worker
// context.
func (p *Pool) ExecFrom(r io.Reader) (err error) {
	p.one(func(cx *Context) {
		err = cx.ExecFrom(r)
	})
	return err
}

// Stop the worker threads, destroy all the contexts in the pool.
// This will release any goroutines waiting on the pool.
// Attempting to use a pool after it has been destroyed will cause
// a runtime panic.
func (p *Pool) Destroy() {
	if !p.Valid {
		return
	}
	close(p.in)
	p.Valid = false
	p.cxs = nil
}

// Grab a free worker and exec callback
func (p *Pool) one(callback func(cx *Context)) {
	if !p.Valid {
		panic("attempt to use a pool after it was destroyed")
	}
	fn := &pfn{
		call: callback,
		done: make(chan bool, 1),
	}
	p.in <- fn
	<-fn.done
	return
}

func (p *Pool) Wait() {
	p.wg.Wait()
}

type ObjectPool struct {
	p       *Pool
	objects []*Object
}

func (op *ObjectPool) DefineFunction(name string, fun interface{}) (err error) {
	for _, o := range op.objects {
		err = o.DefineFunction(name, fun)
		if err != nil {
			return
		}
	}
	return
}

func (op *ObjectPool) DefineObject(name string, proxy interface{}) (Definer, error) {
	op2 := &ObjectPool{op.p, make([]*Object, op.p.n)}
	for i, o := range op.objects {
		o, err := o.DefineObject(name, proxy)
		if err != nil {
			return nil, err
		}
		op2.objects[i] = o.(*Object)
	}
	return op2, nil
}
