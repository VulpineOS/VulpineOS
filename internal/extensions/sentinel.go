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
	ListTrustRecipes(ctx context.Context) ([]SentinelTrustRecipe, error)
	ListMaturityMetrics(ctx context.Context) ([]SentinelMaturityMetric, error)
	ListAssignmentRules(ctx context.Context) ([]SentinelAssignmentRule, error)
	ListSessionTimelines(ctx context.Context, filter SentinelTimelineFilter) ([]SentinelSessionTimeline, error)
	ListOutcomeLabels(ctx context.Context) ([]SentinelOutcomeLabel, error)
	SummarizeOutcomes(ctx context.Context) ([]SentinelOutcomeSummary, error)
	SummarizeProbes(ctx context.Context) ([]SentinelProbeSummary, error)
	Available() bool
}

// SentinelStatus describes the currently registered Sentinel backend.
type SentinelStatus struct {
	Provider        string    `json:"provider"`
	Mode            string    `json:"mode"`
	EventSink       string    `json:"eventSink,omitempty"`
	OutcomeSink     string    `json:"outcomeSink,omitempty"`
	VariantSource   string    `json:"variantSource,omitempty"`
	VariantBundles  int       `json:"variantBundles,omitempty"`
	TrustRecipes    int       `json:"trustRecipes,omitempty"`
	MaturityMetrics int       `json:"maturityMetrics,omitempty"`
	AssignmentRules int       `json:"assignmentRules,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt,omitempty"`
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

// SentinelMaturityThreshold defines the minimum value required for a
// named maturity stage on a single metric.
type SentinelMaturityThreshold struct {
	Stage   string `json:"stage"`
	Minimum int    `json:"minimum"`
}

// SentinelMaturityMetric describes one session-maturity dimension that
// Sentinel uses to decide whether a visitor still looks cold or is
// eligible for warmer trust-building variants.
type SentinelMaturityMetric struct {
	ID             string                      `json:"id"`
	Name           string                      `json:"name"`
	Unit           string                      `json:"unit,omitempty"`
	Description    string                      `json:"description,omitempty"`
	HigherIsBetter bool                        `json:"higherIsBetter,omitempty"`
	Thresholds     []SentinelMaturityThreshold `json:"thresholds,omitempty"`
}

// SentinelAssignmentRule maps maturity gates to a trust recipe and
// variant bundle so experiment outcomes remain attributable.
type SentinelAssignmentRule struct {
	ID                      string   `json:"id"`
	Name                    string   `json:"name"`
	Stage                   string   `json:"stage,omitempty"`
	Priority                int      `json:"priority"`
	DomainScope             []string `json:"domainScope,omitempty"`
	VariantBundleID         string   `json:"variantBundleId,omitempty"`
	TrustRecipeID           string   `json:"trustRecipeId,omitempty"`
	MinSessionAgeSeconds    int      `json:"minSessionAgeSeconds,omitempty"`
	MinSuccessfulVisits     int      `json:"minSuccessfulVisits,omitempty"`
	MinDistinctDays         int      `json:"minDistinctDays,omitempty"`
	MinChallengeFreeRuns    int      `json:"minChallengeFreeRuns,omitempty"`
	MaxRecentHardChallenges int      `json:"maxRecentHardChallenges,omitempty"`
	HoldoutPercent          int      `json:"holdoutPercent,omitempty"`
	Notes                   string   `json:"notes,omitempty"`
}

// SentinelTimelineFilter narrows a timeline query to recent sessions
// or a specific agent/session.
type SentinelTimelineFilter struct {
	SessionID string `json:"sessionId,omitempty"`
	AgentID   string `json:"agentId,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// SentinelTimelineItem is a normalized raw-evidence row derived from
// either a recorded capture event or an outcome label.
type SentinelTimelineItem struct {
	Type            string            `json:"type"`
	Kind            string            `json:"kind,omitempty"`
	Name            string            `json:"name,omitempty"`
	Outcome         string            `json:"outcome,omitempty"`
	ChallengeVendor string            `json:"challengeVendor,omitempty"`
	Source          string            `json:"source,omitempty"`
	Scope           SentinelScope     `json:"scope"`
	Attributes      map[string]string `json:"attributes,omitempty"`
	Payload         json.RawMessage   `json:"payload,omitempty"`
	Timestamp       time.Time         `json:"timestamp"`
}

// SentinelSessionTimeline groups recent evidence by session so
// operators can inspect what a site probed and what happened next.
type SentinelSessionTimeline struct {
	SessionID      string                 `json:"sessionId,omitempty"`
	AgentID        string                 `json:"agentId,omitempty"`
	CitizenID      string                 `json:"citizenId,omitempty"`
	Domain         string                 `json:"domain,omitempty"`
	URL            string                 `json:"url,omitempty"`
	EventCount     int                    `json:"eventCount,omitempty"`
	OutcomeCount   int                    `json:"outcomeCount,omitempty"`
	LastActivityAt time.Time              `json:"lastActivityAt,omitempty"`
	Items          []SentinelTimelineItem `json:"items,omitempty"`
}

// SentinelOutcomeLabel describes one canonical outcome class used by
// Sentinel experiment analysis.
type SentinelOutcomeLabel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Description string `json:"description,omitempty"`
}

// SentinelOutcomeSummary aggregates observed outcome labels so the
// operator can see which classes dominate the current evidence.
type SentinelOutcomeSummary struct {
	Outcome    string    `json:"outcome"`
	Count      int       `json:"count"`
	LastSeenAt time.Time `json:"lastSeenAt,omitempty"`
	Vendors    []string  `json:"vendors,omitempty"`
}

type SentinelProbeSummary struct {
	Domain     string    `json:"domain,omitempty"`
	ScriptURL  string    `json:"scriptUrl,omitempty"`
	ProbeType  string    `json:"probeType,omitempty"`
	API        string    `json:"api,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	LastURL    string    `json:"lastUrl,omitempty"`
	Count      int       `json:"count"`
	LastSeenAt time.Time `json:"lastSeenAt,omitempty"`
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

func (noopSentinelProvider) ListTrustRecipes(ctx context.Context) ([]SentinelTrustRecipe, error) {
	return nil, ErrUnavailable
}

func (noopSentinelProvider) ListMaturityMetrics(ctx context.Context) ([]SentinelMaturityMetric, error) {
	return nil, ErrUnavailable
}

func (noopSentinelProvider) ListAssignmentRules(ctx context.Context) ([]SentinelAssignmentRule, error) {
	return nil, ErrUnavailable
}

func (noopSentinelProvider) ListSessionTimelines(ctx context.Context, filter SentinelTimelineFilter) ([]SentinelSessionTimeline, error) {
	return nil, ErrUnavailable
}

func (noopSentinelProvider) ListOutcomeLabels(ctx context.Context) ([]SentinelOutcomeLabel, error) {
	return nil, ErrUnavailable
}

func (noopSentinelProvider) SummarizeOutcomes(ctx context.Context) ([]SentinelOutcomeSummary, error) {
	return nil, ErrUnavailable
}

func (noopSentinelProvider) SummarizeProbes(ctx context.Context) ([]SentinelProbeSummary, error) {
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
