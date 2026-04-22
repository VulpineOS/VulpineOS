package extensions

import (
	"context"
	"encoding/json"
	"time"
)

const (
	SentinelEventKindBrowserProbe         = "browser_probe"
	SentinelEventKindTransportObservation = "transport_observation"
	SentinelEventKindRuntimeSignal        = "runtime_signal"
	SentinelEventKindTrustActivity        = "trust_activity"
	SentinelEventKindChallengeSignal      = "challenge_signal"

	SentinelOutcomeSuccess       = "success"
	SentinelOutcomeDegraded      = "degraded"
	SentinelOutcomeSoftChallenge = "soft_challenge"
	SentinelOutcomeHardChallenge = "hard_challenge"
	SentinelOutcomeBlock         = "block"
	SentinelOutcomeRetryLoop     = "retry_loop"
	SentinelOutcomeBurn          = "burn"

	SentinelModePublicNoop = "public_noop"
)

// SentinelProvider is the public seam for Sentinel's private capture,
// experiment, and trust-building system. The public build ships a
// nil-safe no-op implementation so VulpineOS compiles and runs without
// any private modules present.
type SentinelProvider interface {
	Status(ctx context.Context) (*SentinelStatus, error)
	RecordEvent(ctx context.Context, event SentinelEvent) error
	RecordOutcome(ctx context.Context, outcome SentinelOutcome) error
	ListVariantBundles(ctx context.Context) ([]SentinelVariantBundle, error)
	Available() bool
}

// SentinelStatus describes the currently registered Sentinel backend.
type SentinelStatus struct {
	Provider       string    `json:"provider"`
	Mode           string    `json:"mode"`
	EventSink      string    `json:"eventSink,omitempty"`
	OutcomeSink    string    `json:"outcomeSink,omitempty"`
	VariantSource  string    `json:"variantSource,omitempty"`
	VariantBundles int       `json:"variantBundles,omitempty"`
	TrustRecipes   int       `json:"trustRecipes,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt,omitempty"`
}

// SentinelScope identifies the agent/session/page scope attached to an
// event or outcome.
type SentinelScope struct {
	CitizenID string `json:"citizenId,omitempty"`
	AgentID   string `json:"agentId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	ContextID string `json:"contextId,omitempty"`
	PageID    string `json:"pageId,omitempty"`
	Domain    string `json:"domain,omitempty"`
	URL       string `json:"url,omitempty"`
	ScriptURL string `json:"scriptUrl,omitempty"`
}

// SentinelEvent is the normalized public schema for private Sentinel
// capture events. Payload remains an opaque JSON blob so the private
// collector can evolve without forcing public type churn for every
// probe subtype.
type SentinelEvent struct {
	Kind       string            `json:"kind"`
	Source     string            `json:"source,omitempty"`
	Name       string            `json:"name"`
	Scope      SentinelScope     `json:"scope"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Payload    json.RawMessage   `json:"payload,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
}

// SentinelOutcome is the normalized public schema for experiment and
// progression labels emitted by Sentinel.
type SentinelOutcome struct {
	Outcome         string            `json:"outcome"`
	Source          string            `json:"source,omitempty"`
	ChallengeVendor string            `json:"challengeVendor,omitempty"`
	Scope           SentinelScope     `json:"scope"`
	Attributes      map[string]string `json:"attributes,omitempty"`
	Timestamp       time.Time         `json:"timestamp"`
}

// SentinelVariantBundle identifies the exact experiment bundle applied
// to a session so outcomes remain attributable.
type SentinelVariantBundle struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	DomainScope        []string  `json:"domainScope,omitempty"`
	BrowserVariant     string    `json:"browserVariant,omitempty"`
	FingerprintVariant string    `json:"fingerprintVariant,omitempty"`
	ProxyVariant       string    `json:"proxyVariant,omitempty"`
	TransportVariant   string    `json:"transportVariant,omitempty"`
	BehaviorVariant    string    `json:"behaviorVariant,omitempty"`
	TrustVariant       string    `json:"trustVariant,omitempty"`
	Weight             int       `json:"weight"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"createdAt,omitempty"`
}

// SentinelTrustRecipe captures a trust-building recipe that can be
// assigned as part of a variant bundle.
type SentinelTrustRecipe struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	DomainScope          []string `json:"domainScope,omitempty"`
	WarmupStrategy       string   `json:"warmupStrategy,omitempty"`
	MinSessionAgeSeconds int      `json:"minSessionAgeSeconds,omitempty"`
	RequiredVisits       int      `json:"requiredVisits,omitempty"`
	ReturnCadence        string   `json:"returnCadence,omitempty"`
	Notes                string   `json:"notes,omitempty"`
}

var defaultSentinelProvider SentinelProvider = noopSentinelProvider{}

type noopSentinelProvider struct{}

func (noopSentinelProvider) Status(ctx context.Context) (*SentinelStatus, error) {
	return nil, ErrUnavailable
}

func (noopSentinelProvider) RecordEvent(ctx context.Context, event SentinelEvent) error {
	return ErrUnavailable
}

func (noopSentinelProvider) RecordOutcome(ctx context.Context, outcome SentinelOutcome) error {
	return ErrUnavailable
}

func (noopSentinelProvider) ListVariantBundles(ctx context.Context) ([]SentinelVariantBundle, error) {
	return nil, ErrUnavailable
}

func (noopSentinelProvider) Available() bool { return false }

// SentinelSnapshot returns the currently registered Sentinel provider
// status plus whether a real provider is present. Public builds return
// available=false and a stable public_noop mode.
func SentinelSnapshot(ctx context.Context) (SentinelStatus, bool, error) {
	provider := Registry.Sentinel()
	if provider == nil || !provider.Available() {
		return SentinelStatus{Mode: SentinelModePublicNoop}, false, nil
	}
	status, err := provider.Status(ctx)
	if err != nil {
		return SentinelStatus{}, true, err
	}
	if status == nil {
		return SentinelStatus{}, true, nil
	}
	out := *status
	if out.Mode == "" {
		out.Mode = SentinelModePublicNoop
	}
	return out, true, nil
}
