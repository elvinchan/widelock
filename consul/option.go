package consul

import "github.com/elvinchan/widelock"

type optionFunc func(widelock.Mutex)

func (f optionFunc) Apply(mutex widelock.Mutex) {
	f(mutex)
}

func WithLostNotify(ch chan<- struct{}) widelock.Option {
	return optionFunc(func(m widelock.Mutex) {
		m.(*mutex).lost = ch
	})
}
