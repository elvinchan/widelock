package widelock

import (
	"context"
	"errors"
	"time"
)

type MutexSet interface {
	NewMutex(key string, opts ...Option) Mutex
	Close() error
}

type Mutex interface {
	Lock(context.Context) error
	TryLock(context.Context) (bool, error)
	Unlock() error
	Extend(context.Context, time.Duration) error
	Valid(context.Context) bool
	Name() string
}

type OptionSet interface {
	Apply(MutexSet)
}

type Option interface {
	Apply(Mutex)
}

var (
	ErrAlreadyClosed    = errors.New("widelock: set already closed")
	ErrAlreadyUnlocked  = errors.New("widelock: already unlocked")
	ErrLockNotHeld      = errors.New("widelock: lock not held")
	ErrLockNotSetExpiry = errors.New("widelock: lock not set expiry")
	ErrInvalidDuration  = errors.New("widelock: invalid duration")
)
