import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import Settings from "./Settings";

describe("Settings page", () => {
  it("shows browser route and mode in the about card", async () => {
    const ws = {
      connected: true,
      call: vi.fn(async (method) => {
        if (method === "config.providers") {
          return {
            providers: [
              {
                id: "anthropic",
                name: "Anthropic (Claude)",
                envVar: "ANTHROPIC_API_KEY",
                defaultModel: "anthropic/claude-sonnet-4-6",
                models: ["anthropic/claude-sonnet-4-6"],
                needsKey: true,
              },
            ],
          };
        }
        if (method === "config.get") {
          return {
            provider: "anthropic",
            model: "claude-sonnet-4-6",
            hasKey: true,
            setupComplete: false,
            defaultBudgetMaxCostUsd: 1.5,
            defaultBudgetMaxTokens: 6000,
          };
        }
        if (method === "status.get") {
          return {
            browser_route: "camoufox",
            browser_route_source: "runtime",
            browser_window: "hidden",
            gateway_running: true,
            sentinel_available: true,
            sentinel_mode: "private_scaffold",
            sentinel_provider: "sentinel-private",
            sentinel_variant_bundles: 1,
            sentinel_trust_recipes: 1,
            sentinel_maturity_metrics: 1,
            sentinel_assignment_rules: 1,
            kernel_headless: false,
            kernel_running: true,
            openclaw_profile_configured: true,
          };
        }
        if (method === "sentinel.get") {
          return {
            available: true,
            variantBundles: [
              { id: "control", name: "Control", enabled: true, weight: 100 },
            ],
            trustRecipes: [
              {
                id: "baseline-warmup",
                name: "Baseline warmup",
                warmupStrategy: "generic_revisit",
              },
            ],
            maturityMetrics: [
              {
                id: "session_age_seconds",
                name: "Session age",
                unit: "seconds",
                thresholds: [{ stage: "warm", minimum: 1800 }],
                description:
                  "How long the identity has existed before higher-trust variants are allowed.",
              },
            ],
            assignmentRules: [
              {
                id: "cold-holdout",
                name: "Cold holdout",
                stage: "cold",
                variantBundleId: "control",
                trustRecipeId: "baseline-warmup",
                holdoutPercent: 100,
              },
            ],
            outcomeLabels: [
              {
                id: "soft_challenge",
                name: "Soft challenge",
                category: "challenge",
                severity: "medium",
                description:
                  "A challenge appeared, but the session may still be recoverable.",
              },
            ],
            outcomeSummary: [
              {
                outcome: "soft_challenge",
                count: 1,
                vendors: ["cloudflare"],
                lastSeenAt: "2026-04-22T15:00:00Z",
              },
            ],
            probeSummary: [
              {
                domain: "example.com",
                scriptUrl: "https://cdn.example.com/fp.js",
                probeType: "canvas_probe",
                api: "toDataURL",
                count: 2,
              },
            ],
            trustActivity: [
              {
                domain: "example.com",
                state: "WARMING",
                count: 3,
                sessionCount: 1,
              },
            ],
            trustEffectiveness: [
              {
                domain: "example.com",
                variantBundleId: "control",
                trustRecipeId: "baseline-warmup",
                warmingCount: 3,
                sessionCount: 2,
                successCount: 1,
                softChallengeCount: 1,
                hardChallengeCount: 0,
                blockCount: 0,
                burnCount: 0,
                effectivenessScore: 2,
              },
            ],
            trustAssets: [
              {
                domain: "example.com",
                variantBundleId: "control",
                trustRecipeId: "baseline-warmup",
                snapshotCount: 1,
                cookieBackedCount: 1,
                storageBackedCount: 1,
                averageCookieCount: 6,
                averageStorageEntryCount: 3,
                averageHoursSinceLastSeen: 12,
                averageTotalSessionsSeen: 4,
                softChallengeCount: 1,
                hardChallengeCount: 0,
                blockCount: 0,
                assetScore: 12,
              },
            ],
            maturityEvidence: [
              {
                domain: "example.com",
                variantBundleId: "control",
                trustRecipeId: "baseline-warmup",
                warmingCount: 3,
                revisitCount: 2,
                distinctDays: 2,
                averageGapHours: 12,
                successCount: 1,
                softChallengeCount: 1,
                hardChallengeCount: 0,
                blockCount: 0,
                maturityScore: 10,
              },
            ],
            transportEvidence: [
              {
                domain: "example.com",
                variantBundleId: "control",
                trustRecipeId: "baseline-warmup",
                rotationCount: 2,
                reasons: ["rate_limit"],
                proxyEndpoints: ["old.example:8080", "new.example:8080"],
                softChallengeCount: 1,
                hardChallengeCount: 1,
                blockCount: 0,
                transportScore: 10,
              },
            ],
            stageSummary: [
              {
                domain: "example.com",
                variantBundleId: "control",
                trustRecipeId: "baseline-warmup",
                currentStage: "warm",
                ruleStage: "cold",
                ruleName: "Cold holdout",
                ruleAligned: true,
                blockingReason: "needs presence across 3 days",
                sessionCount: 2,
                successCount: 1,
                distinctDays: 1,
                challengeFreeRuns: 0,
                sessionAgeSeconds: 7200,
                softChallengeCount: 1,
                hardChallengeCount: 0,
                blockCount: 0,
              },
            ],
            assignmentRecommendations: [
              {
                domain: "example.com",
                variantBundleId: "control",
                trustRecipeId: "baseline-warmup",
                currentStage: "warm",
                action: "promote",
                targetVariantBundleId: "returning-visitor",
                targetTrustRecipeId: "returning-visitor",
                reason: "warm evidence now supports the returning-visitor path",
                priority: "high",
                score: 10,
              },
            ],
            canarySummary: [
              {
                domain: "example.com",
                variantBundleId: "control",
                trustRecipeId: "baseline-warmup",
                canarySessionCount: 4,
                latestOutcome: "hard_challenge",
                latestRecommendationAction: "rotate",
                challengeFreeStreak: 0,
                regressionDelta: -3,
                regressed: true,
              },
            ],
            variantCompareSummary: [
              {
                domain: "example.com",
                variantBundleId: "returning-visitor",
                trustRecipeId: "returning-visitor",
                sessionCount: 4,
                successCount: 2,
                degradedCount: 0,
                softCount: 1,
                hardCount: 1,
                blockCount: 0,
                burnCount: 0,
                totalOutcomes: 4,
                pressureScore: 12,
                canaryStatus: "regressed",
                latestOutcome: "hard_challenge",
              },
            ],
            siteIntelligenceSummary: [
              {
                domain: "example.com",
                topScriptUrl: "https://cdn.example.com/fp.js",
                topProbeFamily: "canvas_probe",
                dominantChallengeVendor: "cloudflare",
                pressureScore: 12,
                activeRecommendationCount: 1,
                latestCanaryStatus: "regressed",
              },
            ],
            probeSequenceSummary: [
              {
                domain: "example.com",
                scriptUrl: "https://cdn.example.com/fp.js",
                sequence: "canvas_probe.toDataURL -> webgl_probe.getParameter",
                stepCount: 2,
                sequenceCount: 2,
                latestOutcome: "hard_challenge",
                latestChallengeVendor: "cloudflare",
                latestCanaryStatus: "regressed",
              },
            ],
            vendorIntelligenceSummary: [
              {
                vendorFamily: "cloudflare",
                scriptHost: "cdn.example.com",
                challengeVendor: "cloudflare",
                domainCount: 2,
                domainSamples: ["example.com", "shop.example.com"],
                topProbeFamily: "canvas_probe",
                pressureScore: 18,
                latestOutcome: "hard_challenge",
                latestCanaryStatus: "regressed",
              },
            ],
            vendorEffectiveness: [
              {
                vendorFamily: "cloudflare",
                variantBundleId: "control",
                trustRecipeId: "baseline-warmup",
                domainCount: 2,
                successCount: 1,
                softChallengeCount: 1,
                hardChallengeCount: 0,
                blockCount: 0,
                burnCount: 0,
                effectivenessScore: 0,
              },
            ],
            vendorUplift: [
              {
                vendorFamily: "cloudflare",
                variantBundleId: "returning-visitor",
                trustRecipeId: "returning-visitor",
                controlVariantBundleId: "control",
                controlTrustRecipeId: "baseline-warmup",
                baselineAvailable: true,
                successRateDeltaPct: 25,
                challengeRateDeltaPct: -25,
                scoreDelta: 8,
                recommendation: "promote",
                confidence: "medium",
              },
            ],
            vendorRollout: [
              {
                vendorFamily: "cloudflare",
                leadingVariantBundleId: "returning-visitor",
                leadingTrustRecipeId: "returning-visitor",
                controlVariantBundleId: "control",
                controlTrustRecipeId: "baseline-warmup",
                baselineAvailable: true,
                armCount: 2,
                scoreDelta: 8,
                successRateDeltaPct: 25,
                challengeRateDeltaPct: -25,
                recommendation: "expand",
                confidence: "medium",
              },
            ],
            trustPlaybook: [
              {
                variantBundleId: "returning-visitor",
                trustRecipeId: "returning-visitor",
                vendorFamilyCount: 2,
                expandCount: 2,
                holdCount: 0,
                collectControlCount: 0,
                rollbackCount: 0,
                averageScoreDelta: 8,
                averageSuccessDeltaPct: 25,
                averageChallengeDeltaPct: -25,
                recommendation: "double-down",
                confidence: "low",
              },
            ],
            experimentGaps: [
              {
                vendorFamily: "cloudflare",
                baselineAvailable: true,
                nonBaselineArmCount: 1,
                leadingVariantBundleId: "returning-visitor",
                leadingTrustRecipeId: "returning-visitor",
                bestConfidence: "low",
                nextAction: "deepen-sample",
              },
            ],
            coherenceDiff: [
              {
                domain: "example.com",
                variantBundleId: "authority-ramp",
                trustRecipeId: "authority-ramp",
                findings: [
                  "warm recipe on cold identity",
                  "route churn on supposedly stable identity",
                ],
                sessionCount: 2,
                softChallengeCount: 1,
                hardChallengeCount: 1,
                blockCount: 0,
                severity: "high",
                score: 15,
              },
            ],
            sitePressure: [
              {
                domain: "example.com",
                challengeVendor: "cloudflare",
                probeCount: 2,
                sessionCount: 2,
                successCount: 1,
                softChallengeCount: 1,
                hardChallengeCount: 0,
                blockCount: 0,
                burnCount: 0,
                pressureScore: 6,
              },
            ],
            patchQueue: [
              {
                domain: "example.com",
                probeType: "canvas_probe",
                api: "toDataURL",
                score: 10,
                priority: "high",
                outcomes: ["soft_challenge"],
                recommendation:
                  "Review canvas surface coherence and pixel-read behavior.",
              },
            ],
            patchInvestment: [
              {
                vendorFamily: "cloudflare",
                candidateCount: 1,
                domainCount: 1,
                sampleDomains: ["example.com"],
                topDomain: "example.com",
                topProbeType: "canvas_probe",
                topApi: "toDataURL",
                topPriority: "high",
                totalPatchScore: 10,
                topRecommendation:
                  "Review canvas surface coherence and pixel-read behavior.",
                rolloutRecommendation: "expand",
                gapAction: "deepen-sample",
                leadingVariantBundleId: "returning-visitor",
                leadingTrustRecipeId: "returning-visitor",
                focus: "patch-first",
                reason:
                  "probe pressure is still the most concrete next engineering investment",
                confidence: "medium",
              },
            ],
            surfaceHotspots: [
              {
                probeType: "canvas_probe",
                api: "toDataURL",
                recommendation:
                  "Review canvas surface coherence and pixel-read behavior.",
                candidateCount: 2,
                familyCount: 2,
                vendorFamilies: ["cloudflare", "datadome"],
                domainCount: 2,
                sampleDomains: ["example.com", "shop.example.com"],
                leadVendorFamily: "cloudflare",
                leadFocus: "patch-first",
                peakPriority: "high",
                totalPatchScore: 18,
                patchFirstCount: 1,
                experimentFirstCount: 1,
                rolloutFirstCount: 0,
                observeCount: 0,
                confidence: "high",
              },
            ],
            experimentSummary: [
              {
                variantBundleId: "control",
                trustRecipeId: "baseline-warmup",
                sessionCount: 2,
                domainCount: 1,
                successCount: 1,
                softChallengeCount: 1,
                hardChallengeCount: 0,
                blockCount: 0,
                burnCount: 0,
                challengeVendors: ["cloudflare"],
              },
            ],
          };
        }
        if (method === "sentinel.timeline") {
          return {
            sessions: [
              {
                sessionId: "session-1",
                agentId: "agent-1",
                domain: "example.com",
                url: "https://example.com",
                eventCount: 1,
                outcomeCount: 1,
                items: [
                  {
                    type: "event",
                    kind: "browser_probe",
                    name: "canvas.toDataURL",
                    variantBundleId: "control",
                    trustRecipeId: "baseline-warmup",
                    attributes: {
                      prior_session_count: "2",
                      distinct_days_seen: "2",
                      hours_since_last_seen: "12.0",
                    },
                  },
                  {
                    type: "outcome",
                    outcome: "soft_challenge",
                    challengeVendor: "cloudflare",
                    variantBundleId: "control",
                    trustRecipeId: "baseline-warmup",
                  },
                ],
              },
            ],
          };
        }
        return {};
      }),
    };

    render(<Settings ws={ws} />);

    expect(
      await screen.findByText("Route: CAMOUFOX (runtime) · GUI"),
    ).toBeInTheDocument();
    expect(screen.getByText("Window: HIDDEN")).toBeInTheDocument();
    expect(screen.getByText("Gateway: RUNNING")).toBeInTheDocument();
    expect(
      screen.getByText("Sentinel: PRIVATE_SCAFFOLD · sentinel-private"),
    ).toBeInTheDocument();
    expect(screen.getByText("Variant bundles: 1")).toBeInTheDocument();
    expect(screen.getByText("Trust recipes: 1")).toBeInTheDocument();
    expect(screen.getByText("Maturity metrics: 1")).toBeInTheDocument();
    expect(screen.getByText("Assignment rules: 1")).toBeInTheDocument();
    expect(screen.getByText("Variant names: Control")).toBeInTheDocument();
    expect(
      screen.getByText("Trust names: Baseline warmup"),
    ).toBeInTheDocument();
    expect(screen.getByText("Session age")).toBeInTheDocument();
    expect(screen.getByText("Cold holdout")).toBeInTheDocument();
    expect(screen.getByText("warm 1800 seconds")).toBeInTheDocument();
    expect(screen.getByText("Experiment board")).toBeInTheDocument();
    expect(screen.getAllByText("Baseline warmup").length).toBeGreaterThan(0);
    expect(screen.getByText("Variant compare board")).toBeInTheDocument();
    expect(screen.getByText("REGRESSED · HARD_CHALLENGE")).toBeInTheDocument();
    expect(screen.getByText("Site intelligence board")).toBeInTheDocument();
    expect(screen.getByText("Probe sequence board")).toBeInTheDocument();
    expect(
      screen.getByText("canvas_probe.toDataURL -> webgl_probe.getParameter"),
    ).toBeInTheDocument();
    expect(screen.getAllByText("hard_challenge").length).toBeGreaterThan(0);
    expect(screen.getByText("Vendor intelligence board")).toBeInTheDocument();
    expect(screen.getAllByText("cloudflare").length).toBeGreaterThan(0);
    expect(screen.getByText("cdn.example.com")).toBeInTheDocument();
    expect(
      screen.getAllByText("example.com, shop.example.com").length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("Vendor effectiveness board")).toBeInTheDocument();
    expect(screen.getByText("Vendor uplift board")).toBeInTheDocument();
    expect(screen.getAllByText("25%").length).toBeGreaterThan(0);
    expect(screen.getAllByText("-25%").length).toBeGreaterThan(0);
    expect(screen.getAllByText("PROMOTE").length).toBeGreaterThan(0);
    expect(screen.getAllByText("MEDIUM").length).toBeGreaterThan(0);
    expect(screen.getByText("Vendor rollout board")).toBeInTheDocument();
    expect(screen.getAllByText("EXPAND").length).toBeGreaterThan(0);
    expect(screen.getByText("Trust playbook board")).toBeInTheDocument();
    expect(screen.getByText("DOUBLE-DOWN")).toBeInTheDocument();
    expect(screen.getAllByText("LOW").length).toBeGreaterThan(0);
    expect(screen.getByText("Experiment gap board")).toBeInTheDocument();
    expect(screen.getAllByText("DEEPEN-SAMPLE").length).toBeGreaterThan(0);
    expect(screen.getByText("Patch investment board")).toBeInTheDocument();
    expect(screen.getByText("PATCH-FIRST")).toBeInTheDocument();
    expect(
      screen.getByText(
        "probe pressure is still the most concrete next engineering investment",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("Surface hotspot board")).toBeInTheDocument();
    expect(
      screen.getByText("1 patch · 1 experiment · 0 rollout"),
    ).toBeInTheDocument();
    expect(screen.getByText("cloudflare, datadome")).toBeInTheDocument();
    expect(screen.getByText("Recent capture timeline")).toBeInTheDocument();
    expect(
      screen.getByText("example.com · 1 events · 1 outcomes"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "browser_probe · canvas.toDataURL · seen 2 sessions · 2 days · gap 12.0h · Control / Baseline warmup",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "soft_challenge · cloudflare · Control / Baseline warmup",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("Probe summary")).toBeInTheDocument();
    expect(screen.getAllByText("canvas_probe").length).toBeGreaterThan(0);
    expect(
      screen.getAllByText("https://cdn.example.com/fp.js").length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("Site pressure board")).toBeInTheDocument();
    expect(screen.getAllByText("example.com").length).toBeGreaterThan(0);
    expect(screen.getAllByText("cloudflare").length).toBeGreaterThan(0);
    expect(screen.getByText("Trust activity board")).toBeInTheDocument();
    expect(screen.getByText("WARMING")).toBeInTheDocument();
    expect(screen.getByText("Trust effectiveness")).toBeInTheDocument();
    expect(screen.getByText("Trust assets")).toBeInTheDocument();
    expect(screen.getByText("Maturity evidence")).toBeInTheDocument();
    expect(screen.getByText("Stage board")).toBeInTheDocument();
    expect(screen.getByText("Cold holdout (cold)")).toBeInTheDocument();
    expect(
      screen.getByText("needs presence across 3 days"),
    ).toBeInTheDocument();
    expect(screen.getByText("Assignment recommendations")).toBeInTheDocument();
    expect(
      screen.getByText("warm evidence now supports the returning-visitor path"),
    ).toBeInTheDocument();
    expect(screen.getByText("Canary board")).toBeInTheDocument();
    expect(screen.getAllByText("REGRESSED").length).toBeGreaterThan(0);
    expect(screen.getByText("ROTATE")).toBeInTheDocument();
    expect(screen.getByText("Transport evidence")).toBeInTheDocument();
    expect(screen.getByText("Coherence diff")).toBeInTheDocument();
    expect(
      screen.getByText(
        "warm recipe on cold identity | route churn on supposedly stable identity",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("Patch queue")).toBeInTheDocument();
    expect(
      screen.getAllByText(
        "Review canvas surface coherence and pixel-read behavior.",
      ).length,
    ).toBeGreaterThan(0);
    expect(screen.getAllByText("HIGH").length).toBeGreaterThan(0);
    expect(screen.getByText("Outcome taxonomy")).toBeInTheDocument();
    expect(screen.getByText("Soft challenge")).toBeInTheDocument();
    expect(screen.getByText("Captured outcomes")).toBeInTheDocument();
    expect(screen.getAllByText("soft_challenge").length).toBeGreaterThan(0);
    expect(screen.getAllByText("cloudflare").length).toBeGreaterThan(0);
    expect(
      screen.getByText("Agent model setup: Not configured"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("OpenClaw profile: Configured"),
    ).toBeInTheDocument();
    expect(screen.getByText("API Key (ANTHROPIC_API_KEY)")).toBeInTheDocument();
    expect(
      screen.getByText(
        "A key is already stored locally. Leave this blank to keep it.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByDisplayValue("1.5")).toBeInTheDocument();
    expect(screen.getByDisplayValue("6000")).toBeInTheDocument();

    fireEvent.change(screen.getByDisplayValue("1.5"), {
      target: { value: "2.25" },
    });
    fireEvent.change(screen.getByDisplayValue("6000"), {
      target: { value: "9000" },
    });
    fireEvent.click(screen.getByText("Save Defaults"));

    await waitFor(() => {
      expect(ws.call).toHaveBeenCalledWith("config.set", {
        defaultBudgetMaxCostUsd: 2.25,
        defaultBudgetMaxTokens: 9000,
      });
    });
  }, 15000);
});
