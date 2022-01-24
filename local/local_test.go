package local

import (
	"context"
	"testing"
	"time"

	"github.com/elvinchan/util-collects/testkit"
	"github.com/elvinchan/widelock"
)

func TestLockUnlock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	set := New()
	t.Run("Simple", func(t *testing.T) {
		t.Parallel()
		m := set.NewMutex("t_simple")
		err := m.Lock(ctx)
		testkit.Assert(t, err == nil)

		now := time.Now()

		go func() {
			// unlock after 1 millisecond
			time.Sleep(time.Millisecond)
			err := m.Unlock()
			testkit.Assert(t, err == nil)
		}()
		err = m.Lock(ctx)
		testkit.Assert(t, err == nil)
		elapsed := time.Since(now)
		testkit.Assert(t, elapsed >= time.Millisecond)
	})

	t.Run("Context", func(t *testing.T) {
		t.Parallel()
		m := set.NewMutex("t_context")
		err := m.Lock(ctx)
		testkit.Assert(t, err == nil)

		now := time.Now()
		ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
		defer cancel()

		finish := make(chan struct{})
		go func() {
			err := m.Lock(ctx)
			testkit.Assert(t, err == context.DeadlineExceeded)
			close(finish)
		}()

		<-finish
		elapsed := time.Since(now)
		testkit.Assert(t, elapsed >= time.Millisecond)

		err = m.Unlock()
		testkit.Assert(t, err == nil)
	})
}

func TestClose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	set := New()
	m := set.NewMutex("t_close")

	err := set.Close()
	testkit.Assert(t, err == nil)

	err = m.Lock(ctx)
	testkit.Assert(t, err == widelock.ErrAlreadyClosed)

	hold, err := m.TryLock(ctx)
	testkit.Assert(t, hold == false)
	testkit.Assert(t, err == widelock.ErrAlreadyClosed)

	err = m.Extend(ctx, 1)
	testkit.Assert(t, err == widelock.ErrAlreadyClosed)

	valid := m.Valid(ctx)
	testkit.Assert(t, valid == false)

	err = set.Close()
	testkit.Assert(t, err == widelock.ErrAlreadyClosed)
}

func TestExtend(t *testing.T) {
	set := New()
	ctx := context.Background()

	t.Run("Expiry", func(t *testing.T) {
		m := set.NewMutex("t_extend",
			WithExpiry(time.Now().Add(time.Millisecond*2)))
		err := m.Lock(ctx)
		testkit.Assert(t, err == nil)

		now := time.Now()
		err = m.Lock(ctx)
		testkit.Assert(t, err == nil)
		elapsed := time.Since(now)
		testkit.Assert(t, elapsed > time.Millisecond*2)
		testkit.Assert(t, elapsed < time.Millisecond*100)
	})
}
