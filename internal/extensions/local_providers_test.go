//go:build !private

package extensions

import (
	"encoding/json"
	"testing"
)

// TestLocalProviders_ZeroOnPublicBuild asserts that on the default
// public build the local provider constructor slots are empty and
// InitWithClient is a complete no-op. This guards against accidental
// public-side registration of optional adapters.
func TestLocalProviders_ZeroOnPublicBuild(t *testing.T) {
	if privateProviders.Vault != nil {
		t.Errorf("privateProviders.Vault should be nil on public build")
	}
	if privateProviders.Audio != nil {
		t.Errorf("privateProviders.Audio should be nil on public build")
	}
	if privateProviders.Mobile != nil {
		t.Errorf("privateProviders.Mobile should be nil on public build")
	}

	// InitWithClient with a non-nil stub client must not mutate the
	// Registry on the public build because every local constructor is nil.
	before := Registry.Credentials()
	InitWithClient(stubJugglerCallable{})
	after := Registry.Credentials()
	if before != after {
		t.Errorf("InitWithClient mutated Registry.Credentials on public build")
	}
}

// stubJugglerCallable is a do-nothing JugglerCallable used only to
// exercise the InitWithClient code path in public-build tests.
type stubJugglerCallable struct{}

func (stubJugglerCallable) Call(sessionID, method string, params interface{}) (json.RawMessage, error) {
	return nil, nil
}
