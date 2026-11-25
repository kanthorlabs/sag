package core

import (
	"fmt"
	"slices"
	"sync"
)

var registry = struct {
	sync.RWMutex
	adapters map[string]Adapter
}{adapters: map[string]Adapter{}}

func Register(a Adapter) {
	registry.Lock()
	defer registry.Unlock()
	kind := a.Kind()
	if _, exists := registry.adapters[kind]; exists {
		panic(fmt.Sprintf("core: adapter kind %q is already registered", kind))
	}
	registry.adapters[kind] = a
}

func Lookup(kind string) (Adapter, bool) {
	registry.RLock()
	defer registry.RUnlock()
	a, ok := registry.adapters[kind]
	return a, ok
}

func Kinds() []string {
	registry.RLock()
	defer registry.RUnlock()
	kinds := make([]string, 0, len(registry.adapters))
	for k := range registry.adapters {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	return kinds
}
