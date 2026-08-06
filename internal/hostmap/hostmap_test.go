package hostmap

import (
	"testing"

	"github.com/go-logr/logr"
)

func TestResolve_SingleCandidate(t *testing.T) {
	got := Resolve([]Candidate{
		{Host: "a.example.com", CNAME: "cname-a", Priority: 0, ClassIndex: 0, Source: "default/a"},
	}, logr.Discard())

	want := map[string]string{"a.example.com": "cname-a"}
	if len(got) != len(want) || got["a.example.com"] != want["a.example.com"] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolve_LowerClassIndexWinsOnEqualPriority(t *testing.T) {
	got := Resolve([]Candidate{
		{Host: "shared.example.com", CNAME: "cname-second", Priority: 0, ClassIndex: 1, Source: "default/second"},
		{Host: "shared.example.com", CNAME: "cname-first", Priority: 0, ClassIndex: 0, Source: "default/first"},
	}, logr.Discard())

	if got["shared.example.com"] != "cname-first" {
		t.Errorf("expected lower ClassIndex to win, got %q", got["shared.example.com"])
	}
}

func TestResolve_HigherPriorityWinsRegardlessOfClassIndex(t *testing.T) {
	got := Resolve([]Candidate{
		{Host: "shared.example.com", CNAME: "cname-first", Priority: 0, ClassIndex: 0, Source: "default/first"},
		{Host: "shared.example.com", CNAME: "cname-second", Priority: 100, ClassIndex: 5, Source: "default/second"},
	}, logr.Discard())

	if got["shared.example.com"] != "cname-second" {
		t.Errorf("expected higher-priority candidate to win despite higher ClassIndex, got %q", got["shared.example.com"])
	}
}

func TestResolve_CrossSourceClassIndexOffset(t *testing.T) {
	// Mirrors how the reconciler merges Ingress and Gateway API candidates:
	// Gateway candidates' ClassIndex is offset by len(ingressClassMappings) so
	// Ingress wins ties by default.
	const ingressClassCount = 1
	ingressCandidate := Candidate{Host: "migrate.example.com", CNAME: "ingress-target", Priority: 0, ClassIndex: 0, Source: "default/ingress"}
	gatewayCandidate := Candidate{Host: "migrate.example.com", CNAME: "gateway-target", Priority: 0, ClassIndex: ingressClassCount + 0, Source: "default/route"}

	got := Resolve([]Candidate{gatewayCandidate, ingressCandidate}, logr.Discard())
	if got["migrate.example.com"] != "ingress-target" {
		t.Errorf("expected Ingress to win by default, got %q", got["migrate.example.com"])
	}

	// Now promote the Gateway candidate via priority annotation - it should win.
	gatewayCandidate.Priority = 100
	got = Resolve([]Candidate{gatewayCandidate, ingressCandidate}, logr.Discard())
	if got["migrate.example.com"] != "gateway-target" {
		t.Errorf("expected Gateway candidate to win after priority promotion, got %q", got["migrate.example.com"])
	}
}

func TestResolve_StableTiebreakOnIdenticalPriorityAndClassIndex(t *testing.T) {
	// Same class index only happens for duplicate claims from within the same
	// source/class; result must be deterministic (lexicographic CNAME) rather
	// than depending on slice order.
	a := Candidate{Host: "dup.example.com", CNAME: "aaa", Priority: 0, ClassIndex: 0, Source: "default/a"}
	b := Candidate{Host: "dup.example.com", CNAME: "bbb", Priority: 0, ClassIndex: 0, Source: "default/b"}

	got1 := Resolve([]Candidate{a, b}, logr.Discard())
	got2 := Resolve([]Candidate{b, a}, logr.Discard())

	if got1["dup.example.com"] != "aaa" || got2["dup.example.com"] != "aaa" {
		t.Errorf("expected stable lexicographic winner \"aaa\" regardless of input order, got %q and %q", got1["dup.example.com"], got2["dup.example.com"])
	}
}

func TestResolve_MultipleHostsIndependent(t *testing.T) {
	got := Resolve([]Candidate{
		{Host: "a.example.com", CNAME: "cname-a", Priority: 0, ClassIndex: 0, Source: "default/a"},
		{Host: "b.example.com", CNAME: "cname-b", Priority: 0, ClassIndex: 0, Source: "default/b"},
	}, logr.Discard())

	if got["a.example.com"] != "cname-a" || got["b.example.com"] != "cname-b" {
		t.Fatalf("expected independent hosts to resolve independently, got %v", got)
	}
}

func TestResolve_Empty(t *testing.T) {
	got := Resolve(nil, logr.Discard())
	if len(got) != 0 {
		t.Errorf("expected empty result for no candidates, got %v", got)
	}
}
