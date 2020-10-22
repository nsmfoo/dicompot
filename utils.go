package dicompot

import (
	"fmt"
	"sync/atomic"
	"time"
)

var idSeq int64

func newUID() string {
	now := time.Now()
	idSeq = now.UnixNano()
	return fmt.Sprintf("%d", atomic.AddInt64(&idSeq, 1))
}

func doassert(cond bool, values ...interface{}) {
	if !cond {
		var s string
		for _, value := range values {
			s += fmt.Sprintf("%v ", value)
		}
		panic(s)
	}
}
