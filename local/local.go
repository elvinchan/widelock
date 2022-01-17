package local

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/elvinchan/widelock"
)

type set struct {
	registry map[string]*mutex
	ttls     heap.Interface // for cleanup expired lock
	ttlAddCh chan struct{}  // signal when add new lock or extend exist lock
	closeCh  chan struct{}
	mu       *sync.Mutex
}

type mutex struct {
	name     string
	set      *set
	ttl      time.Time
	ttlIndex int
	stopCh   chan struct{}

	// options
	duration time.Duration
}

func New() widelock.MutexSet {
	s := &set{
		registry: make(map[string]*mutex),
		ttls:     &mutexHeap{},
		ttlAddCh: make(chan struct{}, 1),
		closeCh:  make(chan struct{}),
		mu:       &sync.Mutex{},
	}
	go s.cleaner()
	return s
}

func (s *set) cleaner() {
	var t *time.Timer
	for {
		s.mu.Lock()
		v := heap.Pop(s.ttls)
		if v == nil {
			s.mu.Unlock()
			select {
			case <-s.closeCh:
				return
			case <-s.ttlAddCh:
				continue
			}
		}
		ttl := v.(*mutex).ttl // ttl may change, put this line in lock block
		s.mu.Unlock()

		d := time.Until(ttl)
		if d > 0 { // not expired yet, wait
			if t == nil {
				// refer to https://github.com/golang/go/issues/12721
				t = time.NewTimer(d)
			} else {
				t.Reset(d)
			}
			select {
			case <-s.closeCh:
				t.Stop() // release timer before exit
				return
			case <-t.C:
			}
		}

		s.mu.Lock()
		m, ok := s.registry[v.(*mutex).name]
		if !ok || m != v || m.ttl.After(time.Now()) {
			// already unlocked or ttl has been extended
			s.mu.Unlock()
			continue
		}
		delete(m.set.registry, m.name)
		close(m.stopCh)
		s.mu.Unlock()
	}
}

func (s *set) NewMutex(name string, opts ...widelock.Option) widelock.Mutex {
	m := &mutex{
		name:     name,
		set:      s,
		ttlIndex: -1,
	}
	for _, opt := range opts {
		opt.Apply(m)
	}
	return m
}

func (s *set) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkNoClose(); err != nil {
		return err
	}
	close(s.closeCh)
	return nil
}

func (s *set) checkNoClose() error {
	select {
	case <-s.closeCh:
		return widelock.ErrAlreadyClosed
	default:
		return nil
	}
}

func (m *mutex) Lock(ctx context.Context) error {
	for {
		succ, stopCh, err := m.acquireLock()
		if err != nil {
			return err
		} else if succ {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stopCh: // signal for previous lock holder to unlock
			continue
		case <-m.set.closeCh:
			return widelock.ErrAlreadyClosed
		}
	}
}

func (m *mutex) TryLock(ctx context.Context) (bool, error) {
	succ, _, err := m.acquireLock()
	return succ, err
}

func (m *mutex) Unlock() error {
	m.set.mu.Lock()
	defer m.set.mu.Unlock()
	if err := m.set.checkNoClose(); err != nil {
		return err
	}
	o, ok := m.set.registry[m.name]
	if !ok || o != m {
		return widelock.ErrLockNotFound
	}
	if m.ttlIndex != -1 {
		heap.Remove(m.set.ttls, m.ttlIndex)
	}
	delete(m.set.registry, m.name)
	close(m.stopCh)
	return nil
}

func (m *mutex) Extend(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return widelock.ErrInvalidDuration
	}
	m.set.mu.Lock()
	defer m.set.mu.Unlock()
	if err := m.set.checkNoClose(); err != nil {
		return err
	}
	o, ok := m.set.registry[m.name]
	if !ok || o != m {
		return widelock.ErrLockNotFound
	}
	if o.ttl.IsZero() {
		return widelock.ErrLockNotSetExpiry
	}
	o.ttl.Add(d)
	if o.ttlIndex == -1 {
		// already pop from heap and waiting to unlock
		heap.Push(m.set.ttls, o)
		// notify ttl add if needed
		select {
		case m.set.ttlAddCh <- struct{}{}:
		default:
		}
	} else {
		heap.Fix(m.set.ttls, o.ttlIndex)
	}
	return nil
}

func (m *mutex) Valid(ctx context.Context) (bool, error) {
	m.set.mu.Lock()
	defer m.set.mu.Unlock()
	if err := m.set.checkNoClose(); err != nil {
		return false, err
	}
	o, ok := m.set.registry[m.name]
	return ok && o == m, nil
}

func (m *mutex) Name() string {
	return m.name
}

func (m *mutex) acquireLock() (bool, <-chan struct{}, error) {
	m.set.mu.Lock()
	defer m.set.mu.Unlock()
	if err := m.set.checkNoClose(); err != nil {
		return false, nil, err
	}
	o, ok := m.set.registry[m.name]
	if ok {
		return false, o.stopCh, nil
	}
	m.stopCh = make(chan struct{})
	if m.duration > 0 {
		m.ttl = time.Now().Add(m.duration)
	}
	m.set.registry[m.name] = m
	if !m.ttl.IsZero() {
		heap.Push(m.set.ttls, m)
		// notify ttl add if needed
		select {
		case m.set.ttlAddCh <- struct{}{}:
		default:
		}
	}
	// lock success
	return true, nil, nil
}
