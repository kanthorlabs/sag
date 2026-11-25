package inventory

import (
	"errors"

	"github.com/kanthorlabs/sag/internal/core"
)

type Status string

const (
	StatusPending     Status = "pending"
	StatusServed      Status = "served"
	StatusUnavailable Status = "unavailable"
)

type Entry struct {
	Name         string
	Kind         string
	Description  string
	Capabilities []string
	Agents       []string
	Resource     core.Resource
	Status       Status
}

type Inventory struct{}

func New() *Inventory {
	return &Inventory{}
}

func (i *Inventory) Add(e Entry) error {
	return errors.ErrUnsupported
}

func (i *Inventory) SetStatus(name string, s Status) error {
	return errors.ErrUnsupported
}

func (i *Inventory) Visible(sub string) []Entry {
	return nil
}

func (i *Inventory) Lookup(sub, name string) (Entry, error) {
	return Entry{}, errors.ErrUnsupported
}
