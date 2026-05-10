package vault

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand"
	"runtime"
	"strings"
)

// FingerprintData holds the key fields we display in the TUI.
type FingerprintData struct {
	UserAgent    string `json:"navigator.userAgent,omitempty"`
	Platform     string `json:"navigator.platform,omitempty"`
	ScreenWidth  int    `json:"screen.width,omitempty"`
	ScreenHeight int    `json:"screen.height,omitempty"`
	Language     string `json:"navigator.language,omitempty"`
}

// HostOS returns the Camoufox OS identifier for the current host.
func HostOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "mac"
	case "windows":
		return "win"
	default:
		return "lin"
	}
}

// GenerateFingerprint returns a deterministic public profile config for the host OS.
// Private builds can replace this path with a richer provider.
func GenerateFingerprint(seed string) (string, error) {
	return generateFallback(seed, HostOS())
}

// generateFallback creates a small deterministic profile for the requested OS.
func generateFallback(seed, hostOS string) (string, error) {
	r := rand.New(rand.NewSource(fingerprintSeed(seed, hostOS)))

	type osProfile struct {
		UA       string
		Platform string
		OsCPU    string
		Lang     string
		Fonts    []string
	}

	profileMap := map[string]osProfile{
		"win": {
			UA:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:134.0) Gecko/20100101 Firefox/134.0",
			Platform: "Win32",
			OsCPU:    "Windows NT 10.0; Win64; x64",
			Lang:     "en-US",
			Fonts:    []string{"Arial", "Calibri", "Cambria", "Consolas", "Segoe UI", "Tahoma", "Times New Roman", "Verdana"},
		},
		"mac": {
			UA:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:134.0) Gecko/20100101 Firefox/134.0",
			Platform: "MacIntel",
			OsCPU:    "Intel Mac OS X 10.15",
			Lang:     "en-US",
			Fonts:    []string{"Arial", "Helvetica", "Helvetica Neue", "Menlo", "Monaco", "San Francisco", "Times New Roman"},
		},
		"lin": {
			UA:       "Mozilla/5.0 (X11; Linux x86_64; rv:134.0) Gecko/20100101 Firefox/134.0",
			Platform: "Linux x86_64",
			OsCPU:    "Linux x86_64",
			Lang:     "en-US",
			Fonts:    []string{"Arial", "DejaVu Sans", "Liberation Mono", "Liberation Sans", "Noto Sans", "Ubuntu"},
		},
	}

	p, ok := profileMap[hostOS]
	if !ok {
		p = profileMap["lin"]
	}
	resolutions := [][2]int{{1920, 1080}, {2560, 1440}, {1366, 768}, {1536, 864}, {1440, 900}}
	res := resolutions[r.Intn(len(resolutions))]

	config := map[string]interface{}{
		"navigator.userAgent":           p.UA,
		"navigator.platform":            p.Platform,
		"navigator.oscpu":               p.OsCPU,
		"navigator.language":            p.Lang,
		"navigator.languages":           p.Lang + "," + p.Lang[:2],
		"navigator.hardwareConcurrency": []int{4, 8, 8, 12, 16}[r.Intn(5)],
		"navigator.maxTouchPoints":      0,
		"screen.width":                  res[0],
		"screen.height":                 res[1],
		"screen.colorDepth":             24,
		"screen.pixelDepth":             24,
		"screen.availWidth":             res[0],
		"screen.availHeight":            res[1] - 48,
		"window.outerWidth":             res[0] - r.Intn(20),
		"window.outerHeight":            res[1] - 100 - r.Intn(40),
		"window.devicePixelRatio":       []float64{1.0, 1.0, 1.5, 2.0}[r.Intn(4)],
		"fonts":                         p.Fonts,
		"canvas:seed":                   r.Uint32(),
		"audio:seed":                    r.Uint32(),
		"fonts:spacing_seed":            r.Uint32(),
	}

	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal fallback fingerprint: %w", err)
	}
	return string(data), nil
}

func fingerprintSeed(seed, hostOS string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(hostOS))
	return int64(h.Sum64())
}

// ParseFingerprint extracts key display fields from a fingerprint JSON blob.
func ParseFingerprint(fpJSON string) (*FingerprintData, error) {
	var fp FingerprintData
	if err := json.Unmarshal([]byte(fpJSON), &fp); err != nil {
		return nil, fmt.Errorf("parse fingerprint: %w", err)
	}
	return &fp, nil
}

// FingerprintSummary returns a human-readable one-liner for a fingerprint.
func FingerprintSummary(fpJSON string) string {
	// Try parsing as Camoufox config format (dot-separated keys)
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(fpJSON), &raw); err != nil {
		return "unknown"
	}

	ua, _ := raw["navigator.userAgent"].(string)
	platform, _ := raw["navigator.platform"].(string)
	sw, _ := raw["screen.width"].(float64)
	sh, _ := raw["screen.height"].(float64)

	osName := "Unknown"
	switch {
	case strings.Contains(platform, "Win"):
		osName = "Windows"
	case strings.Contains(platform, "Mac"):
		osName = "macOS"
	case strings.Contains(platform, "Linux"):
		osName = "Linux"
	}

	browser := "Firefox"
	if strings.Contains(ua, "rv:") {
		// Extract Firefox version
		idx := strings.Index(ua, "rv:")
		if idx > 0 {
			end := strings.IndexByte(ua[idx:], ')')
			if end > 0 {
				browser = "Firefox " + ua[idx+3:idx+end]
			}
		}
	}

	if sw > 0 && sh > 0 {
		return fmt.Sprintf("%s / %s / %dx%d", osName, browser, int(sw), int(sh))
	}
	return fmt.Sprintf("%s / %s", osName, browser)
}
