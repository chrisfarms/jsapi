package jsapi

import(
	"sync"
)

type evalCmd struct {
	source string
	result interface{}
	done chan error
}

type Pool struct {
	cxs []*Context
	eval chan *evalCmd
	wg sync.WaitGroup
}

func NewPool(n int) *Pool {
	p := &Pool{}
	p.cxs = make([]*Context, n)
	p.eval = make(chan *evalCmd)
	for i := 0; i < n; i++ {
		cx := NewContext()
		p.cxs[i] = cx
		p.wg.Add(1)
		go func(cx *Context){
			for {
				select {
				case cmd := <-p.eval:
					cmd.done <-cx.Eval(cmd.source, cmd.result)
				}
			}
		}(cx)
	}
	return p
}

func (p *Pool) DefineFunction(name string, fun interface{}) *Func {
	for _, cx := range p.cxs {
		cx.DefineFunction(name, fun)
	}
	return nil // TODO: should return FuncPool
}

func (p *Pool) DefineObject(name string, proxy interface{}) *Object {
	for _, cx := range p.cxs {
		cx.DefineObject(name, proxy)
	}
	return nil //TODO: should return ObjectPool
}

func (p *Pool) Eval(source string, result interface{}) (err error) {
	cmd := &evalCmd{source, result, make(chan error, 1)}
	p.eval <-cmd
	return <-cmd.done
}

func (p *Pool) Wait() {
	p.wg.Wait()
}

