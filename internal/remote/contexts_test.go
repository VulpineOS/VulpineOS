package remote

import "testing"

func TestContextRegistryTracksAttachDetach(t *testing.T) {
	reg := NewContextRegistry()
	reg.Created("ctx-1")
	reg.Attached("sess-1", "ctx-1", "https://example.com")
	reg.Attached("sess-2", "ctx-1", "https://example.com/page")

	list := reg.List()
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	if list[0].Pages != 2 {
		t.Fatalf("pages = %d, want 2", list[0].Pages)
	}
	if list[0].LastURL != "https://example.com/page" {
		t.Fatalf("lastURL = %q", list[0].LastURL)
	}

	reg.Detached("sess-1")
	list = reg.List()
	if list[0].Pages != 1 {
		t.Fatalf("pages after detach = %d, want 1", list[0].Pages)
	}
}

func TestContextRegistryRemoveClearsSessions(t *testing.T) {
	reg := NewContextRegistry()
	reg.Attached("sess-1", "ctx-1", "https://example.com")
	reg.Removed("ctx-1")
	reg.Detached("sess-1")

	list := reg.List()
	if len(list) != 0 {
		t.Fatalf("len(list) = %d, want 0", len(list))
	}
}

func TestContextRegistryCountsSessionRemapToNewContext(t *testing.T) {
	reg := NewContextRegistry()
	reg.Attached("sess-1", "ctx-1", "https://example.com/one")
	reg.Attached("sess-1", "ctx-2", "https://example.com/two")

	list := reg.List()
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	counts := map[string]int{}
	for _, ctx := range list {
		counts[ctx.ID] = ctx.Pages
	}
	if counts["ctx-1"] != 0 {
		t.Fatalf("ctx-1 pages = %d, want 0", counts["ctx-1"])
	}
	if counts["ctx-2"] != 1 {
		t.Fatalf("ctx-2 pages = %d, want 1", counts["ctx-2"])
	}
}
