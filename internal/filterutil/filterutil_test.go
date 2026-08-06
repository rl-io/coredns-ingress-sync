package filterutil

import "testing"

func TestNamespaceScope_WatchAllByDefault(t *testing.T) {
	scope := NewNamespaceScope("", "")
	if !scope.WatchesAllNamespaces() {
		t.Fatal("expected empty watch env to mean watch all namespaces")
	}
	if !scope.ShouldWatch("anything") {
		t.Error("expected all namespaces to be watched")
	}
	if scope.WatchNamespaces() != nil {
		t.Errorf("expected nil watch list when watching all, got %v", scope.WatchNamespaces())
	}
}

func TestNamespaceScope_WatchAllWithExclusions(t *testing.T) {
	scope := NewNamespaceScope("", "kube-system,excluded-ns")
	if !scope.ShouldWatch("default") {
		t.Error("expected default namespace to be watched")
	}
	if scope.ShouldWatch("kube-system") {
		t.Error("expected kube-system to be excluded")
	}
	if scope.ShouldWatch("excluded-ns") {
		t.Error("expected excluded-ns to be excluded")
	}
}

func TestNamespaceScope_ExplicitWatchList(t *testing.T) {
	scope := NewNamespaceScope("default,staging", "")
	if scope.WatchesAllNamespaces() {
		t.Fatal("expected explicit watch list to not watch all namespaces")
	}
	if !scope.ShouldWatch("default") || !scope.ShouldWatch("staging") {
		t.Error("expected explicitly listed namespaces to be watched")
	}
	if scope.ShouldWatch("production") {
		t.Error("expected unlisted namespace to not be watched")
	}
	got := scope.WatchNamespaces()
	if len(got) != 2 || got[0] != "default" || got[1] != "staging" {
		t.Errorf("expected [default staging], got %v", got)
	}
}

func TestNamespaceScope_ExplicitWatchListWithExclusion(t *testing.T) {
	scope := NewNamespaceScope("default,staging", "staging")
	if !scope.ShouldWatch("default") {
		t.Error("expected default to be watched")
	}
	if scope.ShouldWatch("staging") {
		t.Error("expected staging to be excluded even though explicitly watched")
	}
}

func TestNamespaceScope_WhitespaceInLists(t *testing.T) {
	scope := NewNamespaceScope(" default , staging ", "")
	if !scope.ShouldWatch("default") || !scope.ShouldWatch("staging") {
		t.Error("expected whitespace around namespace names to be trimmed")
	}
}

func TestExcludeSet_BareName(t *testing.T) {
	set := NewExcludeSet("excluded-ingress")
	if !set.IsExcluded("default", "excluded-ingress") {
		t.Error("expected bare name to be excluded in any namespace")
	}
	if !set.IsExcluded("other-ns", "excluded-ingress") {
		t.Error("expected bare name to be excluded regardless of namespace")
	}
	if set.IsExcluded("default", "other-ingress") {
		t.Error("expected non-matching name to not be excluded")
	}
}

func TestExcludeSet_NamespacedName(t *testing.T) {
	set := NewExcludeSet("default/excluded-ingress")
	if !set.IsExcluded("default", "excluded-ingress") {
		t.Error("expected namespace/name to be excluded in that namespace")
	}
	if set.IsExcluded("other-ns", "excluded-ingress") {
		t.Error("expected namespace/name exclusion to not apply to a different namespace")
	}
}

func TestExcludeSet_MixedAndEmpty(t *testing.T) {
	set := NewExcludeSet("bare-name, default/scoped-name")
	if !set.IsExcluded("any-ns", "bare-name") {
		t.Error("expected bare-name to be excluded anywhere")
	}
	if !set.IsExcluded("default", "scoped-name") {
		t.Error("expected default/scoped-name to be excluded in default")
	}
	if set.IsExcluded("other", "scoped-name") {
		t.Error("expected scoped-name exclusion to not leak to other namespace")
	}

	empty := NewExcludeSet("")
	if empty.IsExcluded("default", "anything") {
		t.Error("expected empty exclude env to exclude nothing")
	}
}

func TestExcludeSet_MalformedEntriesIgnored(t *testing.T) {
	set := NewExcludeSet("/missing-ns,missing-name/,,valid-name")
	if !set.IsExcluded("any", "valid-name") {
		t.Error("expected the one valid entry to still be parsed")
	}
}

func TestResolvePriority_Default(t *testing.T) {
	if got := ResolvePriority(nil, "my-priority-key"); got != 0 {
		t.Errorf("expected default priority 0, got %d", got)
	}
	if got := ResolvePriority(map[string]string{}, "my-priority-key"); got != 0 {
		t.Errorf("expected default priority 0 for empty annotations, got %d", got)
	}
}

func TestResolvePriority_ExplicitValue(t *testing.T) {
	annotations := map[string]string{"my-priority-key": "42"}
	if got := ResolvePriority(annotations, "my-priority-key"); got != 42 {
		t.Errorf("expected priority 42, got %d", got)
	}
}

func TestResolvePriority_TrimsWhitespace(t *testing.T) {
	annotations := map[string]string{"my-priority-key": " 7 "}
	if got := ResolvePriority(annotations, "my-priority-key"); got != 7 {
		t.Errorf("expected priority 7, got %d", got)
	}
}

func TestResolvePriority_InvalidValueFallsBackToZero(t *testing.T) {
	annotations := map[string]string{"my-priority-key": "not-a-number"}
	if got := ResolvePriority(annotations, "my-priority-key"); got != 0 {
		t.Errorf("expected fallback priority 0 for invalid value, got %d", got)
	}
}

func TestResolvePriority_EmptyAnnotationKeyDisablesLookup(t *testing.T) {
	annotations := map[string]string{"my-priority-key": "99"}
	if got := ResolvePriority(annotations, ""); got != 0 {
		t.Errorf("expected priority 0 when annotation key is empty, got %d", got)
	}
}

func TestIsFalseLike(t *testing.T) {
	falseValues := []string{"false", "0", "no", "off", "disabled", "FALSE", " False ", "Off"}
	for _, v := range falseValues {
		if !IsFalseLike(v) {
			t.Errorf("expected %q to be false-like", v)
		}
	}

	truthyValues := []string{"true", "1", "yes", "on", "enabled", "", "banana"}
	for _, v := range truthyValues {
		if IsFalseLike(v) {
			t.Errorf("expected %q to not be false-like", v)
		}
	}
}
