package extensions_test

// This integration test lives in package extensions_test so that it
// can import vulpineos/internal/mcp without creating an import cycle
// with the extensions package itself. It verifies that a registered
// CredentialProvider is visible through the MCP tool boundary.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"vulpineos/internal/extensions"
	"vulpineos/internal/juggler"
	"vulpineos/internal/mcp"
)

// deadTransport is a juggler.Transport that never delivers anything.
// It is only used to construct a non-nil *juggler.Client so we can
// exercise HandleToolCallDirect on tool paths that don't actually
// invoke the transport (e.g. vulpine_get_credential, which only
// touches the extensions registry).
type deadTransport struct {
	closed chan struct{}
	once   sync.Once
}

func newDeadTransport() *deadTransport { return &deadTransport{closed: make(chan struct{})} }

func (d *deadTransport) Send(msg *juggler.Message) error {
	return fmt.Errorf("deadTransport: send not supported")
}

func (d *deadTransport) Receive() (*juggler.Message, error) {
	<-d.closed
	return nil, fmt.Errorf("deadTransport: closed")
}

func (d *deadTransport) Close() error {
	d.once.Do(func() { close(d.closed) })
	return nil
}

// fakeCredentialProvider is a minimal provider that returns a single
// hard-coded credential for any siteURL. It exists only for tests.
type fakeCredentialProvider struct {
	cred extensions.Credential
}

func (f *fakeCredentialProvider) Lookup(ctx context.Context, siteURL string) (*extensions.Credential, error) {
	c := f.cred
	return &c, nil
}

func (f *fakeCredentialProvider) Fill(ctx context.Context, credID string, target extensions.FillTarget) error {
	return nil
}

func (f *fakeCredentialProvider) GenerateCode(ctx context.Context, credID string) (string, error) {
	return "000000", nil
}

func (f *fakeCredentialProvider) List(ctx context.Context) ([]extensions.Credential, error) {
	return []extensions.Credential{f.cred}, nil
}

func (f *fakeCredentialProvider) Available() bool { return true }

// TestMCPGetCredentialWithFakeProvider wires a fake provider into the
// global registry and verifies that the MCP tool vulpine_get_credential
// returns its metadata. Registry state is restored in cleanup.
func TestMCPGetCredentialWithFakeProvider(t *testing.T) {
	original := extensions.Registry.Credentials()
	t.Cleanup(func() {
		extensions.Registry.SetCredentials(original)
	})

	fake := &fakeCredentialProvider{
		cred: extensions.Credential{
			ID:       "cred-xyz",
			Site:     "https://example.com",
			Username: "alice@example.com",
			HasTOTP:  true,
			Notes:    "test account",
		},
	}
	extensions.Registry.SetCredentials(fake)

	args, err := json.Marshal(map[string]interface{}{
		"site_url": "https://example.com",
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	client := juggler.NewClient(newDeadTransport())
	t.Cleanup(func() { _ = client.Close() })

	res, err := mcp.HandleToolCallDirect(client, "vulpine_get_credential", args)
	if err != nil {
		t.Fatalf("HandleToolCallDirect: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res)
	}
	if len(res.Content) == 0 {
		t.Fatal("expected content")
	}
	body := res.Content[0].Text
	// Verify the structured JSON body contains the fake credential fields.
	for _, want := range []string{"cred-xyz", "alice@example.com", "test account"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected body to contain %q, got %q", want, body)
		}
	}
	// Parse and check hasTOTP.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if parsed["hasTOTP"] != true {
		t.Errorf("expected hasTOTP=true, got %v", parsed["hasTOTP"])
	}
}

// TestMCPGetCredentialRestoresAfterCleanup verifies that after the
// previous test's t.Cleanup fires, the default unavailable provider is
// back in place — a regression guard against cross-test leakage.
func TestMCPGetCredentialRestoresAfterCleanup(t *testing.T) {
	if extensions.Registry.Credentials().Available() {
		t.Fatal("expected default credential provider to report Available()==false; registry leaked between tests")
	}
	args, _ := json.Marshal(map[string]interface{}{"site_url": "https://example.com"})
	client := juggler.NewClient(newDeadTransport())
	t.Cleanup(func() { _ = client.Close() })
	res, err := mcp.HandleToolCallDirect(client, "vulpine_get_credential", args)
	if err != nil {
		t.Fatalf("HandleToolCallDirect: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError, got success")
	}
	if !strings.Contains(res.Content[0].Text, "credential provider unavailable") {
		t.Errorf("unexpected error text: %q", res.Content[0].Text)
	}
}
