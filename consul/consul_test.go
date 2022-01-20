package consul

import (
	"context"
	"testing"
	"time"

	"github.com/elvinchan/util-collects/testkit"
	"github.com/hashicorp/consul/api"
)

var cc *api.Client

func init() {
	var err error
	cc, err = api.NewClient(api.DefaultConfig())
	if err != nil {
		panic(err)
	}
}

func TestLockUnlock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	set := New(cc)
	t.Run("Simple", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		m := set.NewMutex("simple")
		err := m.Lock(ctx)
		testkit.Assert(t, err == nil)

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
		m := set.NewMutex("context")
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
