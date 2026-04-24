package extensions

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRegistryDefaultsNonNil(t *testing.T) {
	if Registry.Credentials() == nil {
		t.Fatal("Registry.Credentials() is nil")
	}
	if Registry.Audio() == nil {
		t.Fatal("Registry.Audio() is nil")
	}
	if Registry.Mobile() == nil {
		t.Fatal("Registry.Mobile() is nil")
	}
	if Registry.Sentinel() == nil {
		t.Fatal("Registry.Sentinel() is nil")
	}
}

func TestDefaultCredentialProviderUnavailable(t *testing.T) {
	p := Registry.Credentials()
	if p.Available() {
		t.Fatal("default CredentialProvider should report Available() == false")
	}
	ctx := context.Background()

	if _, err := p.Lookup(ctx, "https://example.com"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Lookup: expected ErrUnavailable, got %v", err)
	}
	if err := p.Fill(ctx, "id", FillTarget{PageID: "p", Selector: "#user", Field: "username"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Fill: expected ErrUnavailable, got %v", err)
	}
	if _, err := p.GenerateCode(ctx, "id"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("GenerateCode: expected ErrUnavailable, got %v", err)
	}
	if _, err := p.List(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("List: expected ErrUnavailable, got %v", err)
	}
}

func TestDefaultAudioCapturerUnavailable(t *testing.T) {
	a := Registry.Audio()
	if a.Available() {
		t.Fatal("default AudioCapturer should report Available() == false")
	}
	ctx := context.Background()

	if _, err := a.Start(ctx, CaptureRequest{Format: "pcm", SampleRate: 48000, Channels: 2}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Start: expected ErrUnavailable, got %v", err)
	}
	if err := a.Stop(ctx, "handle"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Stop: expected ErrUnavailable, got %v", err)
	}
	if _, _, err := a.Read(ctx, "handle", 1024); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Read: expected ErrUnavailable, got %v", err)
	}
}

func TestDefaultMobileBridgeUnavailable(t *testing.T) {
	m := defaultMobileBridge
	if m.Available() {
		t.Fatal("default MobileBridge should report Available() == false")
	}
	ctx := context.Background()

	if _, err := m.ListDevices(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListDevices: expected ErrUnavailable, got %v", err)
	}
	if _, err := m.Connect(ctx, "udid"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Connect: expected ErrUnavailable, got %v", err)
	}
	if err := m.Disconnect(ctx, "session"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Disconnect: expected ErrUnavailable, got %v", err)
	}
}

func TestDefaultSentinelProviderUnavailable(t *testing.T) {
	s := Registry.Sentinel()
	if s.Available() {
		t.Fatal("default SentinelProvider should report Available() == false")
	}
	ctx := context.Background()
	if _, err := s.Status(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Status: expected ErrUnavailable, got %v", err)
	}
	if err := s.RecordEvent(ctx, SentinelEvent{
		Kind:      SentinelEventKindBrowserProbe,
		Name:      "canvas.toDataURL",
		Timestamp: time.Now(),
	}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("RecordEvent: expected ErrUnavailable, got %v", err)
	}
	if err := s.RecordOutcome(ctx, SentinelOutcome{
		Outcome:   SentinelOutcomeSoftChallenge,
		Timestamp: time.Now(),
	}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("RecordOutcome: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.ListVariantBundles(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListVariantBundles: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.ListTrustRecipes(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListTrustRecipes: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.ListMaturityMetrics(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListMaturityMetrics: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.ListAssignmentRules(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListAssignmentRules: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.ListSessionTimelines(ctx, SentinelTimelineFilter{Limit: 1}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListSessionTimelines: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.ListOutcomeLabels(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListOutcomeLabels: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeOutcomes(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeOutcomes: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeProbeSequences(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeProbeSequences: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeVendorIntelligence(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeVendorIntelligence: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeVendorEffectiveness(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeVendorEffectiveness: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeVendorUplift(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeVendorUplift: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeVendorRollout(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeVendorRollout: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeTrustPlaybook(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeTrustPlaybook: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeExperimentGaps(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeExperimentGaps: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizePatchQueue(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizePatchQueue: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeTrustRepairQueue(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeTrustRepairQueue: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizePatchInvestment(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizePatchInvestment: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeSurfaceHotspots(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeSurfaceHotspots: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeSurfaceEffectiveness(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeSurfaceEffectiveness: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeSurfaceStrategy(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeSurfaceStrategy: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeTrustStrategy(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeTrustStrategy: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeTrustRecipeStrategy(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeTrustRecipeStrategy: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeTrustRollout(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeTrustRollout: expected ErrUnavailable, got %v", err)
	}
	if _, err := s.SummarizeTrustRolloutDebt(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SummarizeTrustRolloutDebt: expected ErrUnavailable, got %v", err)
	}
}

// TestRegistryConcurrentSetGet runs many goroutines that race on
// setters and readers; the test must pass under `go test -race`.
func TestRegistryConcurrentSetGet(t *testing.T) {
	original := Registry.Credentials()
	t.Cleanup(func() { Registry.SetCredentials(original) })
	originalSentinel := Registry.Sentinel()
	t.Cleanup(func() { Registry.SetSentinel(originalSentinel) })

	var wg sync.WaitGroup
	const N = 100
	for i := 0; i < N; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			Registry.SetCredentials(defaultCredentialProvider)
		}()
		go func() {
			defer wg.Done()
			_ = Registry.Credentials().Available()
		}()
		go func() {
			defer wg.Done()
			Registry.SetSentinel(defaultSentinelProvider)
		}()
		go func() {
			defer wg.Done()
			_ = Registry.Sentinel().Available()
		}()
	}
	wg.Wait()
}
