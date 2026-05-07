package humaninput

import (
	"fmt"
	"math/rand"
	"time"
)

const DefaultWPM = 60

// Keystroke describes one human-paced typing operation.
type Keystroke struct {
	Char         rune
	Delay        time.Duration
	IsCorrection bool
}

// GenerateKeystrokes returns a human-like typing sequence with variable
// inter-key delays, occasional pauses, and rare typo corrections.
func GenerateKeystrokes(text string, wpm int) []Keystroke {
	if wpm <= 0 {
		wpm = DefaultWPM
	}
	baseDelay := 60000 / (wpm * 5)
	if baseDelay < 1 {
		baseDelay = 1
	}

	result := make([]Keystroke, 0, len(text))
	pauseCounter := 5 + rand.Intn(8)
	charsSincePause := 0

	for _, ch := range text {
		delay := baseDelay + int(rand.NormFloat64()*float64(baseDelay)*0.3)
		if delay < 30 {
			delay = 30
		}

		charsSincePause++
		if charsSincePause >= pauseCounter {
			delay *= 2
			charsSincePause = 0
			pauseCounter = 5 + rand.Intn(8)
		}

		if isASCIILetter(ch) && rand.Float64() < 0.05 {
			result = append(result, Keystroke{
				Char:  typoNeighbor(ch),
				Delay: time.Duration(delay) * time.Millisecond,
			})
			result = append(result, Keystroke{
				IsCorrection: true,
				Delay:        time.Duration(150+rand.Intn(100)) * time.Millisecond,
			})
			delay = baseDelay + rand.Intn(max(1, baseDelay/2))
		}

		result = append(result, Keystroke{
			Char:  ch,
			Delay: time.Duration(delay) * time.Millisecond,
		})
	}

	return result
}

// FocusEditableExpression focuses a selector and verifies it is editable.
func FocusEditableExpression(selector string) string {
	return fmt.Sprintf(`(() => {
		const selector = %q;
		const el = document.querySelector(selector);
		if (!el) throw new Error("element not found: " + selector);
		el.focus();
		const editable = el.isContentEditable || el.getAttribute('contenteditable') === 'true' || el.getAttribute('contenteditable') === '';
		if (!('value' in el) && !editable) return "not_input";
		return "ok";
	})()`, selector)
}

// InsertTextExpression appends text at the current caret position of the active
// editable field while dispatching the same input/change notifications used by
// the existing human typing path.
func InsertTextExpression(text string) string {
	return insertTextExpression("document.activeElement", text)
}

// InsertTextIntoSelectorExpression inserts text into a specific editable target.
func InsertTextIntoSelectorExpression(selector, text string) string {
	return insertTextExpression(fmt.Sprintf("document.querySelector(%q)", selector), text)
}

func insertTextExpression(elementExpr, text string) string {
	return fmt.Sprintf(`(() => {
		const el = %s;
		if (!el) return "not_input";
		const text = %q;
		if ('value' in el) {
			const start = typeof el.selectionStart === 'number' ? el.selectionStart : el.value.length;
			const end = typeof el.selectionEnd === 'number' ? el.selectionEnd : el.value.length;
			el.value = el.value.slice(0, start) + text + el.value.slice(end);
			const pos = start + text.length;
			if (typeof el.setSelectionRange === 'function') el.setSelectionRange(pos, pos);
			el.dispatchEvent(new Event('input', {bubbles: true}));
			el.dispatchEvent(new Event('change', {bubbles: true}));
			return "ok";
		}
		const editable = el.isContentEditable || el.getAttribute('contenteditable') === 'true' || el.getAttribute('contenteditable') === '';
		if (editable) {
			document.execCommand('insertText', false, text);
			el.dispatchEvent(new InputEvent('input', {bubbles: true, inputType: 'insertText', data: text}));
			return "ok";
		}
		return "not_input";
	})()`, elementExpr, text)
}

// BackspaceExpression removes the previous character from the active editable
// field. It is used for generated typo corrections.
func BackspaceExpression() string {
	return `(() => {
		const el = document.activeElement;
		if (!el) return "not_input";
		if ('value' in el) {
			const start = typeof el.selectionStart === 'number' ? el.selectionStart : el.value.length;
			const end = typeof el.selectionEnd === 'number' ? el.selectionEnd : el.value.length;
			if (start === 0 && end === 0) return "ok";
			const from = start === end ? Math.max(0, start - 1) : start;
			el.value = el.value.slice(0, from) + el.value.slice(end);
			if (typeof el.setSelectionRange === 'function') el.setSelectionRange(from, from);
			el.dispatchEvent(new Event('input', {bubbles: true}));
			el.dispatchEvent(new Event('change', {bubbles: true}));
			return "ok";
		}
		const editable = el.isContentEditable || el.getAttribute('contenteditable') === 'true' || el.getAttribute('contenteditable') === '';
		if (editable) {
			document.execCommand('delete', false);
			el.dispatchEvent(new InputEvent('input', {bubbles: true, inputType: 'deleteContentBackward'}));
			return "ok";
		}
		return "not_input";
	})()`
}

func isASCIILetter(ch rune) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}

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
