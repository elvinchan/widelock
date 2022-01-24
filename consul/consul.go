package consul

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/elvinchan/widelock"
	"github.com/hashicorp/consul/api"
)

const DefaultKeyPrefix = "widelock/"

type Set struct {
	client   *api.Client
	session  string
	ttls     heap.Interface // for cleanup expired lock
	ttlTopCh chan struct{}  // signal when add new lock or extend exist lock
	closeCh  chan struct{}
	mu       *sync.Mutex

	// options
	prefix    string
	deleteKey bool
}

type mutex struct {
	name     string
	set      *Set
	ttl      time.Time
	ttlIndex int
	stopCh   chan struct{}
	mu       *sync.Mutex

	locker   *api.Lock
	lockOpts *api.LockOptions

	// options
	lost     chan<- struct{}
	duration time.Duration
}

func New(client *api.Client, opts ...widelock.OptionSet) (widelock.MutexSet, error) {
	s := &Set{
		client:   client,
		prefix:   DefaultKeyPrefix,
		ttls:     &mutexHeap{},
		ttlTopCh: make(chan struct{}, 1),
		closeCh:  make(chan struct{}),
		mu:       &sync.Mutex{},
	}
	for _, opt := range opts {
		opt.Apply(s)
	}
	cs := client.Session()
	se := &api.SessionEntry{
		TTL:       api.DefaultLockSessionTTL,
		LockDelay: time.Millisecond,
	}
	if s.deleteKey {
		se.Behavior = api.SessionBehaviorDelete
	}
	var err error
	s.session, _, err = cs.Create(se, nil)
	if err != nil {
		return nil, err
	}
	go s.cleaner()
	go s.renewSessionPeriodic(cs)
	return s, nil
}

func (s *Set) cleaner() {
	var t *time.Timer
	for {
		s.mu.Lock()
		v := heap.Pop(s.ttls)
		if v == nil {
			s.mu.Unlock()
			select {
			case <-s.closeCh:
				return
			case <-s.ttlTopCh:
				continue
			}
		}
		m := v.(*mutex)
		ttl := m.ttl // ttl may change, put this line in lock block
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
			case <-s.ttlTopCh:
				// push v back to ttls
				s.mu.Lock()
				if m.ttlIndex == -1 {
					heap.Push(s.ttls, v)
				}
				s.mu.Unlock()
				continue
			case <-t.C:
			}
		}

		m.mu.Lock()
		if !m.isHeld() || m.ttl.After(time.Now()) {
			m.mu.Unlock()
			continue
		}
		if err := m.locker.Unlock(); err != nil {
			m.mu.Unlock()
			continue // maybe network issue. retry?
		}
		close(m.stopCh)
		m.mu.Unlock()
	}
}

func (s *Set) renewSessionPeriodic(cs *api.Session) {
	ttl, _ := time.ParseDuration(api.DefaultLockSessionTTL)
	waitDur := ttl / 2
	lastRenewTime := time.Now()
	for {
		if time.Since(lastRenewTime) > ttl {
			return
		}
		select {
		case <-time.After(waitDur):
			entry, _, err := cs.Renew(s.session, nil)
			if err != nil {
				// maybe network issue, retry
				waitDur = time.Second
				// TODO: log error
				continue
			}
			if entry == nil {
				// TODO: log ErrSessionExpired
				return
			}

			// Handle the server updating the TTL
			ttl, _ = time.ParseDuration(entry.TTL)
			waitDur = ttl / 2
			lastRenewTime = time.Now()
		case <-s.closeCh:
			return
		}
	}
}

func (s *Set) NewMutex(name string, opts ...widelock.Option) widelock.Mutex {
	m := &mutex{
		name:     name,
		set:      s,
		ttlIndex: -1,
		mu:       &sync.Mutex{},
		lockOpts: &api.LockOptions{
			Key:     s.prefix + name,
			Value:   nil,
			Session: s.session,
		},
	}
	for _, opt := range opts {
		opt.Apply(m)
	}
	return m
}

func (s *Set) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkNoClose(); err != nil {
		return err
	}
	close(s.closeCh)
	_, err := s.client.Session().Destroy(s.session, nil)
	if err != nil {
		return err
	}
	return nil
}

// Cleanup cleanup all the locks with the same prefix in Set, no matter it is
// held or not, even by another Set.
// This may useful when you want to remove all the associate keys in Consul.
func (s *Set) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kv := s.client.KV()
	_, err := kv.DeleteTree(s.prefix, nil)
	return err
}

func (s *Set) checkNoClose() error {
	select {
	case <-s.closeCh:
		return widelock.ErrAlreadyClosed
	default:
		return nil
	}
}

func (m *mutex) Lock(ctx context.Context) error {
	for {
		succ, stopCh, err := m.acquireLock(ctx, true)
		if err != nil {
			return err
		} else if succ {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.set.closeCh:
			return widelock.ErrAlreadyClosed
		case <-stopCh: // signal for previous lock holder to unlock
		}
	}
}

func (m *mutex) TryLock(ctx context.Context) (bool, error) {
	succ, _, err := m.acquireLock(ctx, false)
	return succ, err
}

func (m *mutex) Unlock() error {
	if err := m.set.checkNoClose(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isHeld() {
		return widelock.ErrAlreadyUnlocked
	}
	if err := m.locker.Unlock(); err != nil {
		return err // maybe network issue. retry?
	}
	if m.set.deleteKey {
		if err := m.locker.Destroy(); err != nil {
			return err // maybe network issue. retry?
		}
	}
	m.set.mu.Lock()
	if m.ttlIndex != -1 {
		heap.Remove(m.set.ttls, m.ttlIndex)
	}
	m.set.mu.Unlock()
	close(m.stopCh)
	return nil
}

func (m *mutex) Extend(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return widelock.ErrInvalidDuration
	}
	if err := m.set.checkNoClose(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isHeld() {
		return widelock.ErrAlreadyUnlocked
	}
	if m.ttl.IsZero() {
		return widelock.ErrLockNotSetExpiry
	}
	m.set.mu.Lock()
	m.ttl.Add(d)
	if m.ttlIndex == -1 {
		// already pop from heap and waiting to unlock
		heap.Push(m.set.ttls, m)
	} else {
		heap.Fix(m.set.ttls, m.ttlIndex)
	}
	m.set.mu.Unlock()
	return nil
}

func (m *mutex) Valid(ctx context.Context) bool {
	if err := m.set.checkNoClose(); err != nil {
		return false
	}
	return m.isHeld()
}

func (m *mutex) Name() string {
	return m.name
}

func (m *mutex) acquireLock(ctx context.Context, block bool) (bool, <-chan struct{}, error) {
	if err := m.set.checkNoClose(); err != nil {
		return false, nil, err
	}
	m.mu.Lock()
	if m.isHeld() {
		m.mu.Unlock()
		return false, m.stopCh, nil
	}
	m.lockOpts.LockTryOnce = false
	cl, err := m.set.client.LockOpts(m.lockOpts)
	if err != nil {
		m.mu.Unlock()
		return false, nil, err
	}
	m.stopCh = make(chan struct{})
	m.locker = cl

	if m.duration > 0 {
		m.ttl = time.Now().Add(m.duration)
	}
	if !m.ttl.IsZero() {
		m.set.mu.Lock()
		heap.Push(m.set.ttls, m)
		// notify ttl add if needed
		if m.ttlIndex == 0 {
			select {
			case m.set.ttlTopCh <- struct{}{}:
			default:
			}
		}
		m.set.mu.Unlock()
	}
	m.mu.Unlock()

	lostCh, err := m.locker.Lock(ctx.Done())
	if err != nil {
		return false, m.stopCh, err
	}
	go func() {
		select {
		case <-m.set.closeCh:
			return
		case <-lostCh:
		}
		m.mu.Lock()
		if m.locker == cl && m.isHeld() { // release same mutex
			m.set.mu.Lock()
			if m.ttlIndex != -1 {
				heap.Remove(m.set.ttls, m.ttlIndex)
			}
			m.set.mu.Unlock()
			close(m.stopCh)
		}
		m.mu.Unlock()
	}()
	return true, nil, nil
}

func (m *mutex) isHeld() bool {
	if m.stopCh == nil {
		return false
	}
	select {
	case <-m.stopCh:
		return false
	default:
		return true
	}
}
