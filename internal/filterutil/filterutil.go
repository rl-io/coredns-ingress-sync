// Package filterutil holds parsing and matching helpers shared by the
// Ingress and Gateway API filters (internal/ingress, internal/gatewayapi).
// Both resource kinds need identical namespace-scoping, name-based exclusion,
// and annotation handling, so that logic lives here once.
package filterutil

import (
	"strconv"
	"strings"
)

// NamespaceScope resolves whether a given namespace should be watched, based
// on an optional watch-list and an optional exclude-list.
type NamespaceScope struct {
	watchNamespaces    []string
	watchAllNamespaces bool
	excludeNamespaces  []string
}

// NewNamespaceScope parses the comma-separated watch/exclude namespace
// environment variables. An empty watchNamespacesEnv means "watch all".
func NewNamespaceScope(watchNamespacesEnv, excludeNamespacesEnv string) NamespaceScope {
	scope := NamespaceScope{}

	if watchNamespacesEnv != "" {
		validNamespaces := parseCommaList(watchNamespacesEnv)
		scope.watchNamespaces = validNamespaces
		scope.watchAllNamespaces = len(validNamespaces) == 0
	} else {
		scope.watchAllNamespaces = true
	}

	scope.excludeNamespaces = parseCommaList(excludeNamespacesEnv)

	return scope
}

// ShouldWatch reports whether the given namespace should be processed.
func (s NamespaceScope) ShouldWatch(namespace string) bool {
	if s.watchAllNamespaces {
		for _, ex := range s.excludeNamespaces {
			if ex == namespace {
				return false
			}
		}
		return true
	}

	allowed := false
	for _, ns := range s.watchNamespaces {
		if ns == namespace {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	for _, ex := range s.excludeNamespaces {
		if ex == namespace {
			return false
		}
	}
	return true
}

// WatchNamespaces returns the explicit watch list, or nil if watching all namespaces.
func (s NamespaceScope) WatchNamespaces() []string {
	if s.watchAllNamespaces {
		return nil
	}
	return s.watchNamespaces
}

// WatchesAllNamespaces returns true if watching all namespaces.
func (s NamespaceScope) WatchesAllNamespaces() bool {
	return s.watchAllNamespaces
}

// ExcludeSet resolves whether a given namespaced name is excluded, supporting
// both a bare name (excluded cluster-wide) and a "namespace/name" form.
type ExcludeSet struct {
	names map[string]bool
	byNS  map[string]map[string]bool
}

// NewExcludeSet parses a comma-separated exclude list. Each entry is either a
// bare name (excluded in any namespace) or "namespace/name".
func NewExcludeSet(excludeEnv string) ExcludeSet {
	set := ExcludeSet{
		names: make(map[string]bool),
		byNS:  make(map[string]map[string]bool),
	}

	if excludeEnv == "" {
		return set
	}

	parts := strings.Split(strings.TrimSpace(excludeEnv), ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "/") {
			segs := strings.SplitN(p, "/", 2)
			ns := strings.TrimSpace(segs[0])
			name := strings.TrimSpace(segs[1])
			if ns == "" || name == "" {
				continue
			}
			if _, ok := set.byNS[ns]; !ok {
				set.byNS[ns] = make(map[string]bool)
			}
			set.byNS[ns][name] = true
		} else {
			set.names[p] = true
		}
	}

	return set
}

// IsExcluded reports whether the given namespace/name pair is excluded.
func (e ExcludeSet) IsExcluded(namespace, name string) bool {
	if e.names[name] {
		return true
	}
	if byNS, ok := e.byNS[namespace]; ok && byNS[name] {
		return true
	}
	return false
}

// ResolvePriority returns the explicit priority for an object: the integer
// value of its priority annotation when present and valid, otherwise the
// baseline priority of 0.
func ResolvePriority(annotations map[string]string, annotationKey string) int {
	if annotationKey != "" {
		if val, ok := annotations[annotationKey]; ok {
			if p, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
				return p
			}
		}
	}
	return 0
}

// IsFalseLike returns true if the string represents a false value.
func IsFalseLike(s string) bool {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "false", "0", "no", "off", "disabled":
		return true
	}
	return false
}

// parseCommaList splits a comma-separated string, trimming whitespace and
// dropping empty entries.
func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(strings.ReplaceAll(s, " ", ""), ",") {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
