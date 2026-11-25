package inventory

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/kanthorlabs/sag/internal/core"
)

type Status string

const (
	StatusPending     Status = "pending"
	StatusServed      Status = "served"
	StatusUnavailable Status = "unavailable"
)

var errEmptyName = errors.New("inventory: resource name is empty")

type Entry struct {
	Name         string
	Kind         string
	Description  string
	Capabilities []string
	Agents       []string
	Limits       core.Limits
	Resource     core.Resource
	Status       Status
}

type Inventory struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

func New() *Inventory {
	return &Inventory{entries: map[string]Entry{}}
}

func (i *Inventory) Add(e Entry) error {
	if e.Name == "" {
		return errEmptyName
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, exists := i.entries[e.Name]; exists {
		return fmt.Errorf("inventory: resource %q is already added", e.Name)
	}
	i.entries[e.Name] = cloneEntry(e)
	return nil
}

func (i *Inventory) SetStatus(name string, s Status) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	e, ok := i.entries[name]
	if !ok {
		return fmt.Errorf("inventory: resource %q is unknown", name)
	}
	e.Status = s
	i.entries[name] = e
	return nil
}

func (i *Inventory) Visible(sub string) []Entry {
	i.mu.RLock()
	defer i.mu.RUnlock()
	var visible []Entry
	for _, name := range slices.Sorted(maps.Keys(i.entries)) {
		e := i.entries[name]
		if e.Status == StatusServed && grants(e, sub) {
			visible = append(visible, cloneEntry(e))
		}
	}
	return visible
}

func (i *Inventory) Lookup(sub, name string) (Entry, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	e, ok := i.entries[name]
	if !ok || !grants(e, sub) {
		return Entry{}, core.NewError(core.CodeNotFound, "resource not found")
	}
	if e.Status != StatusServed {
		return Entry{}, core.NewError(core.CodeResourceUnavailable, "resource unavailable")
	}
	return cloneEntry(e), nil
}

func grants(e Entry, sub string) bool {
	return sub != "" && slices.Contains(e.Agents, sub)
}

func cloneEntry(e Entry) Entry {
	e.Agents = slices.Clone(e.Agents)
	e.Capabilities = slices.Clone(e.Capabilities)
	return e
}
