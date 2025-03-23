package cache

import (
	"sync"
)

type item[T any] struct {
	done chan struct{}
	err  error
	val  T
}

// Cache is interface for cache
type Cache[K comparable, V any] interface {
	Get(key K, fn func() (V, error)) (V, error)
}

type Impl[K comparable, T any] struct {
	mu sync.RWMutex
	m  map[K]*item[T]
}

// New makes a Impl instance.
func New[K comparable, T any]() *Impl[K, T] {
	return &Impl[K, T]{
		m: make(map[K]*item[T]),
	}
}

// Get returns the initialized value by key, and make sure the value will be initialized only once.
func (g *Impl[K, T]) Get(key K, fn func() (T, error)) (T, error) {
	g.mu.RLock()
	v, ok := g.m[key]
	g.mu.RUnlock()
	if ok {
		<-v.done
		return v.val, v.err
	}

	g.mu.Lock()
	v, ok = g.m[key]
	if !ok {
		v = &item[T]{
			done: make(chan struct{}),
		}
		g.m[key] = v
	}
	g.mu.Unlock()

	if !ok {
		v.val, v.err = fn()
		if v.err != nil {
			g.mu.Lock()
			delete(g.m, key)
			g.mu.Unlock()
		}
		close(v.done)
	} else {
		<-v.done
	}
	return v.val, v.err
}
