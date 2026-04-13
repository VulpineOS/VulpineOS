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
