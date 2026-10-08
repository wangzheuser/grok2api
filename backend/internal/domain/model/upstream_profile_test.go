package model

import (
	"reflect"
	"testing"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
)

func TestUpstreamProfileOverridesStaticReasoningTable(t *testing.T) {
	ResetUpstreamProfiles()
	t.Cleanup(ResetUpstreamProfiles)

	// Unknown model: none-only until the catalog says otherwise.
	if got := SupportedReasoningEfforts("grok-9.9"); !reflect.DeepEqual(got, []string{ReasoningEffortNone}) {
		t.Fatalf("unknown model efforts = %v", got)
	}
	RegisterUpstreamModelProfile("grok-9.9", UpstreamModelProfile{
		ReasoningEfforts:       []string{"XHigh", "high", "medium", "low", "high", "bogus"},
		DefaultReasoningEffort: "high",
		ContextWindow:          500000,
	})
	want := []string{ReasoningEffortXHigh, ReasoningEffortHigh, ReasoningEffortMedium, ReasoningEffortLow}
	if got := SupportedReasoningEfforts("grok-9.9"); !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog efforts = %v, want %v", got, want)
	}
	if got := SupportedReasoningEfforts("Build/grok-9.9"); !reflect.DeepEqual(got, want) {
		t.Fatalf("prefixed catalog efforts = %v", got)
	}
	if !SupportsReasoningEffort("grok-9.9", "xhigh") || SupportsReasoningEffort("grok-9.9", "max") {
		t.Fatal("menu membership must drive SupportsReasoningEffort")
	}
	// Catalog default wins over the static medium preference.
	if got := DefaultReasoningEffort("grok-9.9"); got != ReasoningEffortHigh {
		t.Fatalf("default = %q", got)
	}
	if got := DefaultReasoningEffortForProvider(account.ProviderBuild, "grok-9.9"); got != ReasoningEffortHigh {
		t.Fatalf("provider default = %q", got)
	}
	// Aliases follow the live menu.
	if base, effort, ok := ParseReasoningModelAlias("grok-9.9-xhigh"); !ok || base != "grok-9.9" || effort != ReasoningEffortXHigh {
		t.Fatalf("alias parse = %q %q %v", base, effort, ok)
	}
	if got := ReasoningAliasPublicIDs("grok-9.9"); !reflect.DeepEqual(got, []string{"grok-9.9-xhigh", "grok-9.9-high", "grok-9.9-medium", "grok-9.9-low"}) {
		t.Fatalf("aliases = %v", got)
	}
	profile, ok := UpstreamProfile("Build/grok-9.9")
	if !ok || profile.ContextWindow != 500000 || !profile.SupportsReasoningEffort || profile.DefaultReasoningEffort != "high" {
		t.Fatalf("profile = %#v ok=%v", profile, ok)
	}
}

func TestUpstreamProfileWithoutMenuKeepsStaticTable(t *testing.T) {
	ResetUpstreamProfiles()
	t.Cleanup(ResetUpstreamProfiles)
	RegisterUpstreamModelProfile("grok-4.5", UpstreamModelProfile{ContextWindow: 256000})
	want := []string{ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh}
	if got := SupportedReasoningEfforts("grok-4.5"); !reflect.DeepEqual(got, want) {
		t.Fatalf("efforts = %v", got)
	}
	if got := DefaultReasoningEffort("grok-4.5"); got != ReasoningEffortMedium {
		t.Fatalf("default = %q", got)
	}
}

func TestUpstreamProfileDefaultDroppedWhenNotInMenu(t *testing.T) {
	profile := NormalizeUpstreamProfile(UpstreamModelProfile{ReasoningEfforts: []string{"low", "high"}, DefaultReasoningEffort: "xhigh"})
	if profile.DefaultReasoningEffort != "" {
		t.Fatalf("default = %q", profile.DefaultReasoningEffort)
	}
}

func TestMinimalIsAKnownTierAndAliasSuffix(t *testing.T) {
	ResetUpstreamProfiles()
	t.Cleanup(ResetUpstreamProfiles)
	if !IsKnownReasoningEffort("minimal") || !IsKnownReasoningEffort("MAX") || IsKnownReasoningEffort("ultra") {
		t.Fatal("known tier vocabulary drifted from grok-build")
	}
	RegisterUpstreamModelProfile("grok-9.8", UpstreamModelProfile{ReasoningEfforts: []string{"minimal", "low", "max"}})
	if base, effort, ok := ParseReasoningModelAlias("grok-9.8-minimal"); !ok || base != "grok-9.8" || effort != ReasoningEffortMinimal {
		t.Fatalf("alias = %q %q %v", base, effort, ok)
	}
	if base, effort, ok := ParseReasoningModelAlias("grok-9.8-max"); !ok || base != "grok-9.8" || effort != ReasoningEffortMax {
		t.Fatalf("alias = %q %q %v", base, effort, ok)
	}
}

func TestGrok47StaticCapabilities(t *testing.T) {
	ResetUpstreamProfiles()
	want := []string{ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh, ReasoningEffortXHigh}
	if got := SupportedReasoningEfforts("grok-4.7"); !reflect.DeepEqual(got, want) {
		t.Fatalf("grok-4.7 efforts = %v", got)
	}
}

func TestBuildProfileDoesNotOverrideOtherProviders(t *testing.T) {
	ResetUpstreamProfiles()
	t.Cleanup(ResetUpstreamProfiles)
	RegisterUpstreamModelProfile("grok-4.6", UpstreamModelProfile{
		ReasoningEfforts: []string{"minimal", "high", "max"}, DefaultReasoningEffort: "max",
	})
	for _, provider := range []account.Provider{account.ProviderWeb, account.ProviderConsole} {
		want := []string{ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh, ReasoningEffortXHigh}
		if got := SupportedReasoningEffortsForProvider(provider, "grok-4.6"); !reflect.DeepEqual(got, want) {
			t.Errorf("%s efforts = %v, want %v", provider, got, want)
		}
		for _, name := range []string{"grok-4.6", provider.ModelNamespace() + "/grok-4.6"} {
			if got := DefaultReasoningEffortForProvider(provider, name); got != ReasoningEffortMedium {
				t.Errorf("%s default = %q", provider, got)
			}
		}
		if got := DefaultReasoningEffort(provider.ModelNamespace() + "/grok-4.6"); got != ReasoningEffortMedium {
			t.Errorf("%s prefixed default = %q", provider, got)
		}
		if _, effort, ok := ParseReasoningModelAlias(provider.ModelNamespace() + "/grok-4.6-xhigh"); !ok || effort != ReasoningEffortXHigh {
			t.Errorf("%s static xhigh alias disappeared", provider)
		}
		if _, _, ok := ParseReasoningModelAlias(provider.ModelNamespace() + "/grok-4.6-minimal"); ok {
			t.Errorf("%s advertised Build-only minimal alias", provider)
		}
	}
	if got := DefaultReasoningEffortForProvider(account.ProviderBuild, "grok-4.6"); got != ReasoningEffortMax {
		t.Fatalf("Build default = %q", got)
	}
}
