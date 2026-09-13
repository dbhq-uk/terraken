package main

import "testing"

// buildVersion falls back to the module version Go recorded, because the
// goreleaser ldflag never reaches a binary built by "go install".
func TestBuildVersionPrefersTheLdflag(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })

	version = "1.2.3"
	if got := buildVersion(); got != "1.2.3" {
		t.Errorf("buildVersion() = %q, want the ldflag value 1.2.3", got)
	}
}

func TestBuildVersionNeverReturnsEmpty(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })

	// Under "go test" the main module reads as "(devel)", so this
	// exercises the fallback's own fallback: it must degrade to "dev"
	// rather than to an empty string, or --version prints nothing.
	version = "dev"
	if got := buildVersion(); got == "" {
		t.Error("buildVersion() returned an empty string")
	}
}
