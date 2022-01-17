package local

import (
	"time"

	"github.com/elvinchan/widelock"
)

type optionFunc func(widelock.Mutex)

func (f optionFunc) Apply(mutex widelock.Mutex) {
	f(mutex)
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
