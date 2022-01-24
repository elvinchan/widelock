package consul

import (
	"time"

	"github.com/elvinchan/widelock"
)

type optionSetFunc func(widelock.MutexSet)

func (f optionSetFunc) Apply(set widelock.MutexSet) {
	f(set)
}

// WithKeyPrefix create a Set option to set the prefix of key of Consul.
// Default is `widelock:`
func WithKeyPrefix(prefix string) widelock.OptionSet {
	return optionSetFunc(func(ms widelock.MutexSet) {
		ms.(*Set).prefix = prefix
	})
}

// WithDeleteKey create a Set option to specify behavior to delete key of
// unlocked locker and all keys in Set after closed.
// This would slightly increase load of unlock, but if the locker would be used
// only once in most cases, enable this is helpful to reduce memory usage of
// Consul server.
func WithDeleteKey() widelock.OptionSet {
	return optionSetFunc(func(ms widelock.MutexSet) {
		ms.(*Set).deleteKey = true
	})
}

type optionFunc func(widelock.Mutex)

func (f optionFunc) Apply(mutex widelock.Mutex) {
	f(mutex)
}

func WithLostNotify(ch chan<- struct{}) widelock.Option {
	return optionFunc(func(m widelock.Mutex) {
		m.(*mutex).lost = ch
	})
}

// WithExpiry set expiry for lock. It would be covered by duration if set.
func WithExpiry(e time.Time) widelock.Option {
	return optionFunc(func(m widelock.Mutex) {
		m.(*mutex).ttl = e
	})
}

// WithDuration set duration for sustaining the lock. It start from lock really
// obtained.
func WithDuration(d time.Duration) widelock.Option {
	return optionFunc(func(m widelock.Mutex) {
		m.(*mutex).duration = d
	})
}
