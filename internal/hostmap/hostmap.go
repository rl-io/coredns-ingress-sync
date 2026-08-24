// Package hostmap resolves hostname -> target CNAME rewrite rules from a set
// of candidates, potentially declared by more than one source (Ingress,
// Gateway API HTTPRoute). It is the single place that decides who wins when
// multiple candidates claim the same hostname.
package hostmap

import "github.com/go-logr/logr"

// Candidate is a single hostname claim contributed by some source object
// (an Ingress rule, or an HTTPRoute hostname).
type Candidate struct {
	Host  string
	CNAME string
	// Priority is the resolved priority for this candidate; higher wins.
	Priority int
	// ClassIndex is the config-order index of the class/mapping this
	// candidate came from; lower wins ties. Callers merging candidates from
	// more than one source should offset each source's indices so the
	// combined space stays free of unintended collisions (e.g. Gateway API
	// candidates continuing after all Ingress class indices), which makes the
	// first-configured source the default winner when nothing is annotated.
	ClassIndex int
	// Source identifies the originating object (namespace/name), used only
	// for tie-break logging.
	Source string
}

// beats reports whether candidate c should win over the current best b.
// Higher priority wins; on equal priority the lower ClassIndex wins, so the
// first-configured class/source is the safe default when nothing is
// annotated.
func (c Candidate) beats(b Candidate) bool {
	if c.Priority != b.Priority {
		return c.Priority > b.Priority
	}
	if c.ClassIndex != b.ClassIndex {
		return c.ClassIndex < b.ClassIndex
	}
	// Same priority and same class index (only possible for the same
	// class/source, hence identical CNAME). Fall back to a stable
	// lexicographic pick so the output never flaps across reconciles.
	return c.CNAME < b.CNAME
}

// ResolveCandidates returns the winning Candidate for each hostname, picking
// via Candidate.beats. Unlike Resolve, it preserves the winning candidate's
// Source, which callers can use to report which object caused a hostname to
// be added or repointed. logger is used to record (at V(1)) cases where two
// candidates with different CNAMEs contend for the same hostname; pass
// logr.Discard() if logging isn't needed.
func ResolveCandidates(candidates []Candidate, logger logr.Logger) map[string]Candidate {
	winners := make(map[string]Candidate)

	for _, candidate := range candidates {
		existing, ok := winners[candidate.Host]
		if !ok {
			winners[candidate.Host] = candidate
			continue
		}
		if candidate.beats(existing) {
			// Only log when the resolved targets actually differ; same-class
			// duplicates produce identical CNAMEs and are not noteworthy.
			if candidate.CNAME != existing.CNAME {
				logger.V(1).Info("Hostname claimed by multiple sources; higher-priority candidate wins",
					"host", candidate.Host,
					"winner", candidate.Source, "winnerCNAME", candidate.CNAME, "winnerPriority", candidate.Priority,
					"loser", existing.Source, "loserCNAME", existing.CNAME, "loserPriority", existing.Priority)
			}
			winners[candidate.Host] = candidate
		}
	}

	return winners
}

// Resolve returns a map of hostname -> target CNAME, picking the winning
// candidate for each hostname via Candidate.beats. logger is used to record
// (at V(1)) cases where two candidates with different CNAMEs contend for the
// same hostname; pass logr.Discard() if logging isn't needed.
func Resolve(candidates []Candidate, logger logr.Logger) map[string]string {
	winners := ResolveCandidates(candidates, logger)

	result := make(map[string]string, len(winners))
	for host, w := range winners {
		result[host] = w.CNAME
	}
	return result
}
