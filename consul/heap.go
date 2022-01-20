package consul

type mutexHeap []*mutex

func (mh mutexHeap) Len() int {
	return len(mh)
}

func (mh mutexHeap) Swap(i int, j int) {
	if mh.Len() <= 1 {
		return
	}
	mh[i], mh[j] = mh[j], mh[i]
	mh[i].ttlIndex = i
	mh[j].ttlIndex = j
}

func (mh mutexHeap) Less(i int, j int) bool {
	// Pop will give us the earliest expiry mutex
	if mh.Len() <= 1 {
		return false
	}
	return mh[i].ttl.Before(mh[j].ttl)
}

func (mh *mutexHeap) Pop() interface{} {
	l := mh.Len()
	if l == 0 {
		return nil
	}
	m := (*mh)[l-1]
	m.ttlIndex = -1
	(*mh) = (*mh)[:l-1]
	return m
}

func (mh *mutexHeap) Push(x interface{}) {
	m := x.(*mutex)
	m.ttlIndex = len(*mh)
	*mh = append(*mh, m)
}
