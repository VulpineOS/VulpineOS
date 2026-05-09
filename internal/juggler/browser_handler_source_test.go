package juggler

import (
	"os"
	"strings"
	"testing"
)

func TestBrowserEnableUsesSingleInflightEnable(t *testing.T) {
	data, err := os.ReadFile("../../additions/juggler/protocol/BrowserHandler.js")
	if err != nil {
		t.Fatalf("read BrowserHandler.js: %v", err)
	}
	source := string(data)

	required := []string{
		"this._enablePromise = null;",
		"if (this._enablePromise)",
		"this._enablePromise = this._doEnable({attachToDefaultContext, userPrefs});",
		"async _doEnable({attachToDefaultContext, userPrefs})",
	}
	for _, needle := range required {
		if !strings.Contains(source, needle) {
			t.Fatalf("BrowserHandler.js missing %q", needle)
		}
	}

	assign := strings.Index(source, "this._enablePromise = this._doEnable({attachToDefaultContext, userPrefs});")
	await := strings.Index(source[assign:], "await this._enablePromise;")
	if assign == -1 || await == -1 {
		t.Fatal("Browser.enable does not await the shared in-flight enable promise")
	}
}
