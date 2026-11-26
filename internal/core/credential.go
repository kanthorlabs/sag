package core

import (
	"context"
	"slices"

	"github.com/kanthorlabs/sag/internal/secret"
)

type CredentialKind interface {
	CredentialPrompt() string
	ValidateCredential(cred secret.Credential) (CredentialNotes, error)
	CheckCredential(ctx context.Context, cfg ResourceConfig, cred secret.Credential) error
}

type CredentialNotes struct {
	LoopbackHost bool
}

func CreatableKinds() []string {
	registry.RLock()
	defer registry.RUnlock()
	var kinds []string
	for k, a := range registry.adapters {
		if _, ok := a.(CredentialKind); ok {
			kinds = append(kinds, k)
		}
	}
	slices.Sort(kinds)
	return kinds
}
