package extensions

import (
	"context"
	"errors"
	"fmt"
)

// ValidFillFields is the set of accepted FillTarget.Field values. Any
// other value is rejected by FillTarget.Validate.
var ValidFillFields = map[string]bool{
	"username": true,
	"password": true,
}

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
	// Fill writes the credential's value into the targeted form field.
	// The provider owns the actual injection mechanism so plaintext never
	// crosses this interface boundary.
	Fill(ctx context.Context, credID string, target FillTarget) error
	// GenerateCode returns the current TOTP code for a credential, if it has a TOTP secret.
	GenerateCode(ctx context.Context, credID string) (string, error)
	// List returns all credentials for the active citizen, or empty if none.
	List(ctx context.Context) ([]Credential, error)
	// Available reports whether the provider has a real backing implementation.
	Available() bool
}

// FillTarget describes where a credential field should be injected. It
// names a browser page, an optional frame, a CSS selector identifying the
// target field, and whether the username or password slot is being filled.
type FillTarget struct {
	PageID   string // browser page identifier
	FrameID  string // optional frame identifier
	Selector string // CSS selector identifying the target field
	Field    string // "username" or "password"
}

// Validate checks that the FillTarget has a recognized Field value. It
// is called by provider implementations at the top of Fill so callers
// fail fast on typos rather than silently injecting into an unintended
// slot.
func (t FillTarget) Validate() error {
	if t.Field == "" {
		return fmt.Errorf("extensions: FillTarget.Field is required")
	}
	if !ValidFillFields[t.Field] {
		return fmt.Errorf("extensions: FillTarget.Field %q is not one of username/password", t.Field)
	}
	return nil
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

func (noopCredentialProvider) Fill(ctx context.Context, credID string, target FillTarget) error {
	if err := target.Validate(); err != nil {
		return err
	}
	return ErrUnavailable
}

func (noopCredentialProvider) GenerateCode(ctx context.Context, credID string) (string, error) {
	return "", ErrUnavailable
}

func (noopCredentialProvider) List(ctx context.Context) ([]Credential, error) {
	return nil, ErrUnavailable
}

func (noopCredentialProvider) Available() bool { return false }
