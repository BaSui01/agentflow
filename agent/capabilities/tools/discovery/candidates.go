package discovery

// ScoredCandidate is the minimal ordering data for selecting a capability owner.
type ScoredCandidate struct {
	ID    string
	Score float64
	Load  float64
}

// BestCandidate selects the highest-scoring candidate, breaking ties by lower load.
func BestCandidate(candidates []ScoredCandidate) (ScoredCandidate, bool) {
	if len(candidates) == 0 {
		return ScoredCandidate{}, false
	}

	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.Score > best.Score || candidate.Score == best.Score && candidate.Load < best.Load {
			best = candidate
		}
	}
	return best, true
}

// CountAssignmentsForOwner counts how many assignments point to ownerID.
func CountAssignmentsForOwner(assignments map[string]string, ownerID string) int {
	count := 0
	for _, id := range assignments {
		if id == ownerID {
			count++
		}
	}
	return count
}
