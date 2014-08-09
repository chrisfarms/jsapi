package jsapi

import(
	"sync"
	"io"
)

type pfn struct {
	call func(cx *Context)
	done chan bool
}


type Pool struct {
	cxs []*Context
	in chan *pfn
	wg sync.WaitGroup
	n int // readonly pool size
	Valid bool // is this pool active
}

// Create a pool of worker contexts.
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
		go func(cx *Context){
			for {
				select {
				case fn,ok := <-p.in:
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
func (p *Pool) DefineFunction(name string, fun interface{}) {
	for _, cx := range p.cxs {
		cx.DefineFunction(name, fun)
	}
}

// Create an object in ALL contexts within the pool.
// See context's description for more details.
func (p *Pool) DefineObject(name string, proxy interface{}) Definer {
	op := &ObjectPool{p, make([]*Object, p.n)}
	for i, cx := range p.cxs {
		op.objects[i] = cx.DefineObject(name, proxy).(*Object)
	}
	return op
}

// Execute source js in the first available worker context and return
// the result of the expression to result.
func (p *Pool) Eval(source string, result interface{}) (err error) {
	p.one(func(cx *Context){
		err = cx.Eval(source, result)
	})
	return err
}

// Execute source js in the first available worker context.
// Errors are returned but the value of the expression is discarded.
func (p *Pool) Exec(source string) (err error) {
	p.one(func(cx *Context){
		err = cx.Exec(source)
	})
	return err
}

// Execute js from a file in the next available worker context.
func (p *Pool) ExecFile(filename string) (err error) {
	p.one(func(cx *Context){
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
	p.one(func(cx *Context){
		err = cx.ExecFrom(r)
	})
	return err
}

// Stop the worker threads, destroy all the contexts in the pool.
// This will release any goroutines waiting on the pool.
// Attempting to use a pool after it has been destroyed will cause
// a runtime panic.
func (p *Pool) Destroy() {
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
	p.in <-fn
	<-fn.done
	return
}

func (p *Pool) Wait() {
	p.wg.Wait()
}

type ObjectPool struct {
	p *Pool
	objects []*Object
}

func (op *ObjectPool) DefineFunction(name string, fun interface{}) {
	for _, o := range op.objects {
		o.DefineFunction(name, fun)
	}
}

func (op *ObjectPool) DefineObject(name string, proxy interface{}) Definer {
	op2 := &ObjectPool{op.p, make([]*Object, op.p.n)}
	for i, o := range op.objects {
		op2.objects[i] = o.DefineObject(name, proxy).(*Object)
	}
	return op2
}
