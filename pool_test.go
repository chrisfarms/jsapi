package jsapi

import(
	"testing"
)

const(
	delay = 10
	script = `1+1`
)


func BenchmarkEvalSngl(b *testing.B) {
	cx := NewContext()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB){
		for pb.Next() {
			var result interface{}
			err := cx.Eval(script, &result)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkEvalPool(b *testing.B) {
	cx := NewPool(16)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB){
		for pb.Next() {
			var result interface{}
			err := cx.Eval(script, &result)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

