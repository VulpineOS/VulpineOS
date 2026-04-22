package extensionstest

import (
	"context"
	"sync"
	"testing"
	"time"

	"vulpineos/internal/extensions"
)

// TestFakesNoRaceUnderConcurrentSet spawns a pack of goroutines that
// hammer every fake provider's read methods while other goroutines
// mutate the fake through the Set* helpers. Must be run with -race;
// any data race on shared fields will trip the detector and fail the
// test.
func TestFakesNoRaceUnderConcurrentSet(t *testing.T) {
	cred := &FakeCredentialProvider{
		AvailableFlag: true,
		Cred: extensions.Credential{
			ID:       "c1",
			Site:     "https://example.com",
			Username: "alice",
		},
	}
	audio := &FakeAudioCapturer{
		AvailableFlag: true,
		Handle:        extensions.CaptureHandle{ID: "h1", Format: "pcm"},
		Chunk:         []byte{1, 2, 3},
	}
	mobile := &FakeMobileBridge{
		AvailableFlag: true,
		Devices: []extensions.MobileDevice{
			{UDID: "udid-1", Name: "Test iPhone"},
		},
	}
	sentinel := &FakeSentinelProvider{
		AvailableFlag: true,
		StatusValue: extensions.SentinelStatus{
			Provider: "sentinel-private",
			Mode:     "scaffold",
		},
		VariantBundles: []extensions.SentinelVariantBundle{
			{ID: "control", Name: "Control", Enabled: true, Weight: 100},
		},
		TrustRecipes: []extensions.SentinelTrustRecipe{
			{ID: "baseline-warmup", Name: "Baseline warmup", WarmupStrategy: "generic_revisit"},
		},
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	const iters = 50

	// 50 writers, 50 readers per fake = 400 goroutines total.
	for i := 0; i < iters; i++ {
		wg.Add(8)
		// Credential writers + readers.
		go func(i int) {
			defer wg.Done()
			cred.SetCred(extensions.Credential{
				ID:       "c1",
				Site:     "https://example.com",
				Username: "alice",
			})
			cred.SetAvailable(i%2 == 0)
			cred.SetTOTP("123456")
		}(i)
		go func() {
			defer wg.Done()
			_, _ = cred.Lookup(ctx, "https://example.com")
			_, _ = cred.List(ctx)
			_ = cred.Available()
			_, _ = cred.GenerateCode(ctx, "c1")
			_ = cred.Fill(ctx, "c1", extensions.FillTarget{
				PageID:   "p1",
				Selector: "#user",
				Field:    "username",
			})
			_ = cred.RecordedFills()
		}()
		// Audio writers + readers.
		go func(i int) {
			defer wg.Done()
			audio.SetAvailable(i%2 == 0)
			audio.SetHandle(extensions.CaptureHandle{ID: "h1", Format: "pcm"})
			audio.SetChunk([]byte{byte(i)}, i%2 == 0)
		}(i)
		go func() {
			defer wg.Done()
			_, _ = audio.Start(ctx, extensions.CaptureRequest{Format: "pcm"})
			_ = audio.Stop(ctx, "h1")
			_, _, _ = audio.Read(ctx, "h1", 1024)
			_ = audio.LastStartRequest()
			_ = audio.Available()
		}()
		// Mobile writers + readers.
		go func(i int) {
			defer wg.Done()
			mobile.SetAvailable(i%2 == 0)
			mobile.SetDevices([]extensions.MobileDevice{{UDID: "udid-1"}})
			mobile.SetSession(extensions.MobileSession{UDID: "udid-1", CDPEndpoint: "ws://localhost:9222", Protocol: "cdp"})
		}(i)
		go func() {
			defer wg.Done()
			_, _ = mobile.ListDevices(ctx)
			_, _ = mobile.Connect(ctx, "udid-1")
			_ = mobile.Disconnect(ctx, "session-1")
			_ = mobile.Available()
		}()
		// Sentinel writers + readers.
		go func(i int) {
			defer wg.Done()
			sentinel.SetAvailable(i%2 == 0)
			sentinel.SetStatus(extensions.SentinelStatus{
				Provider: "sentinel-private",
				Mode:     "scaffold",
			})
			sentinel.SetVariantBundles([]extensions.SentinelVariantBundle{
				{ID: "control", Name: "Control", Enabled: true, Weight: 100 + i},
			})
			sentinel.SetTrustRecipes([]extensions.SentinelTrustRecipe{
				{ID: "baseline-warmup", Name: "Baseline warmup", WarmupStrategy: "generic_revisit"},
			})
			sentinel.SetMaturityMetrics([]extensions.SentinelMaturityMetric{
				{ID: "session_age_seconds", Name: "Session age"},
			})
			sentinel.SetAssignmentRules([]extensions.SentinelAssignmentRule{
				{ID: "cold-holdout", Name: "Cold holdout"},
			})
			sentinel.SetSessionTimelines([]extensions.SentinelSessionTimeline{
				{SessionID: "session-1", Items: []extensions.SentinelTimelineItem{{Type: "event", Name: "canvas.toDataURL"}}},
			})
		}(i)
		go func(i int) {
			defer wg.Done()
			_, _ = sentinel.Status(ctx)
			_ = sentinel.RecordEvent(ctx, extensions.SentinelEvent{
				Kind:      extensions.SentinelEventKindBrowserProbe,
				Name:      "canvas.toDataURL",
				Timestamp: time.Now(),
			})
			_ = sentinel.RecordOutcome(ctx, extensions.SentinelOutcome{
				Outcome:   extensions.SentinelOutcomeSoftChallenge,
				Timestamp: time.Now(),
			})
			_, _ = sentinel.ListVariantBundles(ctx)
			_, _ = sentinel.ListTrustRecipes(ctx)
			_, _ = sentinel.ListMaturityMetrics(ctx)
			_, _ = sentinel.ListAssignmentRules(ctx)
			_, _ = sentinel.ListSessionTimelines(ctx, extensions.SentinelTimelineFilter{Limit: 1})
			_ = sentinel.RecordedEvents()
			_ = sentinel.RecordedOutcomes()
			_ = sentinel.Available()
		}(i)
	}

	wg.Wait()
}
