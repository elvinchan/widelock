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

	set, err := New(cc)
	testkit.Assert(t, err == nil)
	defer func() {
		err := set.Close()
		testkit.Assert(t, err == nil)
	}()
	t.Run("Simple", func(t *testing.T) {
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

	set, err := New(cc)
	testkit.Assert(t, err == nil)

	se, _, err := cc.Session().Info(set.(*Set).session, nil)
	testkit.Assert(t, err == nil)
	testkit.Assert(t, se != nil)

	err = set.Close()
	testkit.Assert(t, err == nil)

	se, _, err = cc.Session().Info(set.(*Set).session, nil)
	testkit.Assert(t, err == nil)
	testkit.Assert(t, se == nil)
}

func TestOptionSet(t *testing.T) {
	t.Parallel()

	t.Run("WithKeyPrefix", func(t *testing.T) {
		ctx := context.Background()
		set, err := New(cc, WithKeyPrefix("widelock/option_set"))
		testkit.Assert(t, err == nil)
		m := set.NewMutex("t_key_prefix")
		err = m.Lock(ctx)
		testkit.Assert(t, err == nil)

		exist := existConsulKey(t, set.(*Set).prefix+m.Name())
		testkit.Assert(t, exist)

		err = m.Unlock()
		testkit.Assert(t, err == nil)
	})

	t.Run("WithDeleteKey", func(t *testing.T) {
		ctx := context.Background()
		set, err := New(cc, WithDeleteKey())
		testkit.Assert(t, err == nil)
		m := set.NewMutex("t_delete_key")
		err = m.Lock(ctx)
		testkit.Assert(t, err == nil)

		exist := existConsulKey(t, set.(*Set).prefix+m.Name())
		testkit.Assert(t, exist)

		err = m.Unlock()
		testkit.Assert(t, err == nil)

		exist = existConsulKey(t, set.(*Set).prefix+m.Name())
		testkit.Assert(t, !exist)
	})
}

func TestCleanup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	set, err := New(cc, WithKeyPrefix("widelock/cleanup/"))
	testkit.Assert(t, err == nil)
	m := set.NewMutex("t_close")
	err = m.Lock(ctx)
	testkit.Assert(t, err == nil)

	originalSet := set.(*Set)
	kv := originalSet.client.KV()
	kvPair, _, err := kv.Get(originalSet.prefix+m.Name(), nil)
	testkit.Assert(t, err == nil)
	testkit.Assert(t, kvPair != nil)
	testkit.Assert(t, kvPair.Key == originalSet.prefix+m.Name())

	err = originalSet.Cleanup()
	testkit.Assert(t, err == nil)

	kvPair, _, err = kv.Get(originalSet.prefix+m.Name(), nil)
	testkit.Assert(t, err == nil)
	testkit.Assert(t, kvPair == nil)
}

func existConsulKey(t *testing.T, name string) bool {
	pair, _, err := cc.KV().Get(name, nil)
	testkit.Assert(t, err == nil)
	return pair != nil
}
