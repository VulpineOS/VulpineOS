package extensions

import (
	"context"
	"errors"
)

// ErrUnavailable is returned by default provider stubs to indicate that
// the feature has no backing implementation in this build.
var ErrUnavailable = errors.New("extensions: feature unavailable")

// CredentialProvider is the generic interface for looking up and using
// credentials associated with the active citizen. The public surface
// intentionally never exposes raw passwords; implementations are free
// to inject secrets through their own secure channel.
type CredentialProvider interface {
	// Lookup returns a credential matching siteURL, or nil if none.
	Lookup(ctx context.Context, siteURL string) (*Credential, error)
	// GenerateCode returns the current TOTP code for a credential, if it has a TOTP secret.
	GenerateCode(ctx context.Context, credID string) (string, error)
	// List returns all credentials for the active citizen, or empty if none.
	List(ctx context.Context) ([]Credential, error)
	// Available reports whether the provider has a real backing implementation.
	Available() bool
}

// Credential is the public metadata view of a stored credential. The
// password itself is never exposed via this interface.
type Credential struct {
	ID       string
	Site     string
	Username string
	HasTOTP  bool
	// NOTE: password is never exposed via this interface.
	Notes string
}

// defaultCredentialProvider is the no-op stub used when no provider has
// been registered. All methods return ErrUnavailable.
var defaultCredentialProvider CredentialProvider = noopCredentialProvider{}

type noopCredentialProvider struct{}

func (noopCredentialProvider) Lookup(ctx context.Context, siteURL string) (*Credential, error) {
	return nil, ErrUnavailable
}

func (noopCredentialProvider) GenerateCode(ctx context.Context, credID string) (string, error) {
	return "", ErrUnavailable
}

func (noopCredentialProvider) List(ctx context.Context) ([]Credential, error) {
	return nil, ErrUnavailable
}

func (noopCredentialProvider) Available() bool { return false }
