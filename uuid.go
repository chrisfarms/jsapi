package jsapi

import (
	"sync"
)

var counterLock sync.Mutex
var counterValue int = 0

// returns an id guarenteed to be unique within the package
// currently just a simple counter
func uid() (id int) {
	counterLock.Lock()
	counterValue += 1
	id = counterValue
	counterLock.Unlock()
	return id
}
