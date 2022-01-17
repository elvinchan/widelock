package local

import (
	"container/heap"
	"testing"
	"time"

	"github.com/elvinchan/util-collects/testkit"
)

func TestMutexHeap(t *testing.T) {
	t.Parallel()

	mh := mutexHeap{
		{ttl: time.Unix(5, 0)},
		{ttl: time.Unix(2, 0)},
		{ttl: time.Unix(4, 0)},
		{ttl: time.Unix(3, 0)},
	}

	m2 := mh[2]
	heap.Init(&mh)

	v := heap.Pop(&mh)
	testkit.Assert(t, v.(*mutex).ttl.Equal(time.Unix(2, 0)))

	heap.Push(&mh, &mutex{ttl: time.Unix(1, 0)})
	v = heap.Pop(&mh)
	testkit.Assert(t, v.(*mutex).ttl.Equal(time.Unix(1, 0)))

	m2.ttl = time.Unix(1, 0)
	heap.Fix(&mh, 2)
	v = heap.Pop(&mh)
	testkit.Assert(t, v.(*mutex) == m2)
}
