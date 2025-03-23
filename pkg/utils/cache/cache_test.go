package cache

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	key = "foo"
	val = "bar"
)

func TestDo(t *testing.T) {
	g := New[string, string]()
	v, err := g.Get(key, func() (string, error) {
		return val, nil
	})
	require.Nil(t, err)
	require.Equal(t, val, v)
}

func TestParallelDoSuccess(t *testing.T) {
	var (
		wg    sync.WaitGroup
		g     = New[string, string]()
		count = int32(0)
	)
	//nolint:unparam // no need to return error
	fn := func() (string, error) {
		atomic.AddInt32(&count, 1)
		return val, nil
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := g.Get(key, fn)
			assert.Nil(t, err)
			assert.Equal(t, val, v)
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), count)
}

func TestParallelDoFail(t *testing.T) {
	var (
		wg    sync.WaitGroup
		g     = New[string, string]()
		count = int32(0)
		aErr  = fmt.Errorf("aha")
	)

	fn := func() (string, error) {
		atomic.AddInt32(&count, 1)
		return "", aErr
	}

	const loop = 10000
	for i := 0; i < loop; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			time.Sleep(time.Millisecond * time.Duration(i%2))
			_, err := g.Get(key, fn)
			assert.Equal(t, aErr, err)
		}(i)
	}
	wg.Wait()
	// count must bigger than 2 because of sleep
	require.True(t, count >= 2 && count < loop, fmt.Sprintf("actual count: %v", count))
}
