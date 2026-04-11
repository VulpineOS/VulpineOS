package mcp

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"

	"vulpineos/internal/juggler"
)

// --- Human-like interaction tools ---

// handleHumanClick moves the mouse along a realistic bezier curve path to the
// target coordinates, then performs a click. The path includes overshoot and
// micro-jitter to simulate organic human mouse movement.
func handleHumanClick(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID string  `json:"sessionId"`
		X         float64 `json:"x"`
		Y         float64 `json:"y"`
		Speed     string  `json:"speed"` // "slow", "normal", "fast"
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	if p.Speed == "" {
		p.Speed = "normal"
	}

	// Get current mouse position (default 0,0 if unknown)
	fromX, fromY := 0.0, 0.0

	path := generateHumanPath(fromX, fromY, p.X, p.Y, p.Speed)

	// Dispatch mousemove events along the path
	for _, pt := range path {
		time.Sleep(time.Duration(pt.DelayMs) * time.Millisecond)
		_, err := client.Call(p.SessionID, "Page.dispatchMouseEvent", map[string]interface{}{
			"type": "mouseMoved", "x": pt.X, "y": pt.Y,
			"button": 0, "clickCount": 0, "modifiers": 0, "buttons": 0,
		})
		if err != nil {
			return errorResult(err), nil
		}
	}

	// Small pause before clicking (50-100ms)
	time.Sleep(time.Duration(50+rand.Intn(50)) * time.Millisecond)

	// mousedown at final position
	_, err := client.Call(p.SessionID, "Page.dispatchMouseEvent", map[string]interface{}{
		"type": "mousedown", "x": p.X, "y": p.Y,
		"button": 0, "clickCount": 1, "modifiers": 0, "buttons": 1,
	})
	if err != nil {
		return errorResult(err), nil
	}

	// Dwell time (80-150ms)
	time.Sleep(time.Duration(80+rand.Intn(70)) * time.Millisecond)

	// mouseup
	_, err = client.Call(p.SessionID, "Page.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseup", "x": p.X, "y": p.Y,
		"button": 0, "clickCount": 1, "modifiers": 0, "buttons": 0,
	})
	if err != nil {
		return errorResult(err), nil
	}

	return textResult(fmt.Sprintf("Human-clicked at (%v, %v) with %s speed (%d path points)", p.X, p.Y, p.Speed, len(path))), nil
}

// pathPoint represents a single point along a mouse movement path.
type pathPoint struct {
	X       float64
	Y       float64
	DelayMs int
}

// generateHumanPath creates a bezier curve path from (fromX, fromY) to (toX, toY)
// with realistic overshoot, micro-jitter, and bell-shaped velocity profile.
func generateHumanPath(fromX, fromY, toX, toY float64, speed string) []pathPoint {
	dist := math.Hypot(toX-fromX, toY-fromY)

	// Choose point count and duration based on speed + Fitts' law adjustment
	var minDuration, maxDuration int
	var minPoints, maxPoints int

	switch speed {
	case "fast":
		minDuration, maxDuration = 200, 400
		minPoints, maxPoints = 8, 15
	case "slow":
		minDuration, maxDuration = 800, 1500
		minPoints, maxPoints = 25, 40
	default: // "normal"
		minDuration, maxDuration = 400, 800
		minPoints, maxPoints = 15, 25
	}

	// Fitts' law: longer movements take more time
	fittsScale := 1.0 + math.Log2(1+dist/100)*0.15
	totalDuration := float64(minDuration+rand.Intn(maxDuration-minDuration)) * fittsScale
	numPoints := minPoints + rand.Intn(maxPoints-minPoints+1)

	// Generate random bezier control points for curvature
	// Control points are offset perpendicular to the movement direction
	dx := toX - fromX
	dy := toY - fromY

	// Perpendicular direction
	perpX := -dy
	perpY := dx
	perpLen := math.Hypot(perpX, perpY)
	if perpLen > 0 {
		perpX /= perpLen
		perpY /= perpLen
	}

	// Randomized curvature (offset control points by up to 30% of distance)
	curveAmount := dist * 0.3
	cp1Offset := (rand.Float64()*2 - 1) * curveAmount
	cp2Offset := (rand.Float64()*2 - 1) * curveAmount

	// Two control points for cubic bezier
	cp1X := fromX + dx*0.33 + perpX*cp1Offset
	cp1Y := fromY + dy*0.33 + perpY*cp1Offset
	cp2X := fromX + dx*0.66 + perpX*cp2Offset
	cp2Y := fromY + dy*0.66 + perpY*cp2Offset

	// Add overshoot: extend past target by 5-15px then correct back
	overshootDist := 5.0 + rand.Float64()*10.0
	overshootAngle := math.Atan2(toY-fromY, toX-fromX)
	overshootX := toX + math.Cos(overshootAngle)*overshootDist
	overshootY := toY + math.Sin(overshootAngle)*overshootDist

	// Sample points along the cubic bezier
	mainPoints := numPoints - 3 // Reserve 3 points for overshoot correction
	if mainPoints < 5 {
		mainPoints = 5
	}

	points := make([]pathPoint, 0, numPoints)

	for i := 0; i < mainPoints; i++ {
		t := float64(i) / float64(mainPoints-1)

		// Cubic bezier: B(t) = (1-t)^3*P0 + 3(1-t)^2*t*P1 + 3(1-t)*t^2*P2 + t^3*P3
		// But end at overshoot point instead of target
		endX := overshootX
		endY := overshootY
		if i == mainPoints-1 {
			// Last main point is the overshoot position
			endX = overshootX
			endY = overshootY
		}

		mt := 1 - t
		x := mt*mt*mt*fromX + 3*mt*mt*t*cp1X + 3*mt*t*t*cp2X + t*t*t*endX
		y := mt*mt*mt*fromY + 3*mt*mt*t*cp1Y + 3*mt*t*t*cp2Y + t*t*t*endY

		// Add micro-jitter (Gaussian, +/- 1-2px)
		x += rand.NormFloat64() * 1.5
		y += rand.NormFloat64() * 1.5

		// Bell-shaped velocity profile: slower at start and end, faster in middle
		// Use sine-based timing
		velocityFactor := math.Sin(t * math.Pi)
		if velocityFactor < 0.1 {
			velocityFactor = 0.1
		}

		// Inverse: slower velocity = longer delay
		baseDelay := totalDuration / float64(numPoints)
		delay := baseDelay / velocityFactor
		if delay < 5 {
			delay = 5
		}

		points = append(points, pathPoint{
			X:       math.Round(x*10) / 10,
			Y:       math.Round(y*10) / 10,
			DelayMs: int(delay),
		})
	}

	// Overshoot correction: move back from overshoot to actual target
	correctionSteps := 3
	for i := 1; i <= correctionSteps; i++ {
		t := float64(i) / float64(correctionSteps)
		x := overshootX + (toX-overshootX)*t + rand.NormFloat64()*0.5
		y := overshootY + (toY-overshootY)*t + rand.NormFloat64()*0.5
		delay := 20 + rand.Intn(30) // Quick correction movements

		points = append(points, pathPoint{
			X:       math.Round(x*10) / 10,
			Y:       math.Round(y*10) / 10,
			DelayMs: delay,
		})
	}

	return points
}

// handleHumanType types text with realistic human cadence including variable
// inter-key intervals, occasional pauses, and rare typo corrections.
func handleHumanType(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID string `json:"sessionId"`
		Text      string `json:"text"`
		WPM       int    `json:"wpm"` // words per minute, default 60
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	if p.WPM <= 0 {
		p.WPM = 60
	}

	keystrokes := humanTypeDelays(p.Text, p.WPM)
	typed := 0

	for _, ks := range keystrokes {
		// Pre-key delay
		time.Sleep(time.Duration(ks.DelayMs) * time.Millisecond)

		key := string(ks.Char)

		if ks.IsCorrection {
			// Type Backspace to delete the typo
			_, err := client.Call(p.SessionID, "Page.dispatchKeyEvent", map[string]interface{}{
				"type": "keydown", "key": "Backspace", "modifiers": 0,
			})
			if err != nil {
				return errorResult(err), nil
			}
			time.Sleep(time.Duration(60+rand.Intn(40)) * time.Millisecond)
			_, err = client.Call(p.SessionID, "Page.dispatchKeyEvent", map[string]interface{}{
				"type": "keyup", "key": "Backspace", "modifiers": 0,
			})
			if err != nil {
				return errorResult(err), nil
			}

			// Brief pause after noticing typo
			time.Sleep(time.Duration(100+rand.Intn(150)) * time.Millisecond)
			continue
		}

		// keydown
		_, err := client.Call(p.SessionID, "Page.dispatchKeyEvent", map[string]interface{}{
			"type": "keydown", "key": key, "modifiers": 0,
		})
		if err != nil {
			return errorResult(err), nil
		}

		// Dwell time (80-120ms)
		time.Sleep(time.Duration(80+rand.Intn(40)) * time.Millisecond)

		// keyup
		_, err = client.Call(p.SessionID, "Page.dispatchKeyEvent", map[string]interface{}{
			"type": "keyup", "key": key, "modifiers": 0,
		})
		if err != nil {
			return errorResult(err), nil
		}

		typed++
	}

	return textResult(fmt.Sprintf("Human-typed %d characters at ~%d WPM", len(p.Text), p.WPM)), nil
}

// keystroke represents a single key event in a human typing sequence.
type keystroke struct {
	Char         rune
	DelayMs      int
	IsCorrection bool // true = this is a Backspace to fix a typo
}

// humanTypeDelays generates realistic inter-key delays with occasional pauses
// and typo corrections for the given text and typing speed.
func humanTypeDelays(text string, wpm int) []keystroke {
	// Average 5 characters per word
	baseDelay := 60000 / (wpm * 5) // ms per character

	var result []keystroke
	pauseCounter := 5 + rand.Intn(8) // chars until next pause
	charsSincePause := 0

	for _, ch := range text {
		delay := baseDelay + int(rand.NormFloat64()*float64(baseDelay)*0.3)
		if delay < 30 {
			delay = 30
		}

		// Occasional longer pause (simulates thinking or re-reading)
		charsSincePause++
		if charsSincePause >= pauseCounter {
			delay *= 2
			charsSincePause = 0
			pauseCounter = 5 + rand.Intn(8)
		}

		// 5% chance of typo + correction (skip for spaces and special chars)
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' {
			if rand.Float64() < 0.05 {
				// Type a wrong character first
				wrongChar := typoNeighbor(ch)
				result = append(result, keystroke{
					Char:    wrongChar,
					DelayMs: delay,
				})
				// Then backspace to correct
				result = append(result, keystroke{
					IsCorrection: true,
					DelayMs:      150 + rand.Intn(100),
				})
				// The correct character follows with a slightly longer delay
				delay = baseDelay + rand.Intn(baseDelay/2)
			}
		}

		result = append(result, keystroke{
			Char:    ch,
			DelayMs: delay,
		})
	}

	return result
}

// typoNeighbor returns a plausible typo character (adjacent key on QWERTY layout).
func typoNeighbor(ch rune) rune {
	neighbors := map[rune]string{
		'a': "sqwz", 'b': "vngh", 'c': "xvdf", 'd': "sfce", 'e': "wrd",
		'f': "dgcv", 'g': "fhtb", 'h': "gjyn", 'i': "uok", 'j': "hkum",
		'k': "jlio", 'l': "kop", 'm': "njk", 'n': "bmhj", 'o': "iplk",
		'p': "ol", 'q': "wa", 'r': "etf", 's': "adwx", 't': "rgy",
		'u': "yij", 'v': "cfgb", 'w': "qeas", 'x': "zscd", 'y': "tuh",
		'z': "xas",
	}

	lower := ch
	isUpper := ch >= 'A' && ch <= 'Z'
	if isUpper {
		lower = ch + 32
	}

	if adj, ok := neighbors[lower]; ok && len(adj) > 0 {
		picked := rune(adj[rand.Intn(len(adj))])
		if isUpper {
			picked -= 32
		}
		return picked
	}
	return ch
}

// handleHumanScroll scrolls the page with realistic inertial decay: an initial
// large scroll followed by progressively smaller increments with micro-pauses.
func handleHumanScroll(client *juggler.Client, args json.RawMessage) (*ToolCallResult, error) {
	var p struct {
		SessionID string  `json:"sessionId"`
		DeltaY    float64 `json:"deltaY"` // total scroll amount (positive = down)
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return errorResult(err), nil
	}

	// Break into 3-6 scroll events with inertial decay
	numSteps := 3 + rand.Intn(4) // 3-6 steps
	decayFactor := 0.6 + rand.Float64()*0.2 // 60-80% of previous

	remaining := p.DeltaY
	totalScrolled := 0.0
	scrollEvents := 0

	for i := 0; i < numSteps && math.Abs(remaining) > 2; i++ {
		// First step is largest, subsequent steps decay
		var delta float64
		if i == 0 {
			// Initial scroll: 40-60% of total
			fraction := 0.4 + rand.Float64()*0.2
			delta = p.DeltaY * fraction
		} else {
			delta = remaining * (1 - decayFactor)
		}

		// Add slight randomness to each step
		delta += rand.NormFloat64() * 5

		// Clamp to remaining
		if math.Abs(delta) > math.Abs(remaining) {
			delta = remaining
		}

		_, err := client.Call(p.SessionID, "Page.dispatchWheelEvent", map[string]interface{}{
			"x": 400, "y": 300,
			"deltaX": 0, "deltaY": math.Round(delta), "deltaZ": 0,
			"modifiers": 0,
		})
		if err != nil {
			return errorResult(err), nil
		}

		remaining -= delta
		totalScrolled += delta
		scrollEvents++

		// Micro-pause between scroll events (50-150ms)
		if i < numSteps-1 {
			time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond)
		}
	}

	return textResult(fmt.Sprintf("Human-scrolled %v pixels in %d steps", math.Round(totalScrolled), scrollEvents)), nil
}
