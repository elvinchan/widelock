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
	Valid(context.Context) (bool, error)
	Name() string
}

type Option interface {
	Apply(Mutex)
}

var (
	ErrAlreadyClosed    = errors.New("widelock: set already closed")
	ErrAlreadyUnlocked  = errors.New("widelock: already unlocked")
	ErrLockNotFound     = errors.New("widelock: lock not found")
	ErrLockNotSetExpiry = errors.New("widelock: lock not set expiry")
	ErrInvalidDuration  = errors.New("widelock: invalid duration")
)
