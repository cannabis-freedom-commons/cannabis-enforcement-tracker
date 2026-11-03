package research

// factsVerified describes only the retained facts: missing quotations already dropped by
// Confirm do not block verification, and open questions about missing details do not either.
func factsVerified(facts []Fact) bool {
	if len(facts) == 0 {
		return false
	}
	for _, fact := range facts {
		if !fact.confirmed {
			return false
		}
	}
	return true
}

func uncheckedFacts(facts []Fact) []Fact {
	copy := append([]Fact(nil), facts...)
	for i := range copy {
		copy[i].confirmed = false
	}
	return copy
}

func updateStatus(previous string, checked bool) string {
	if previous == "disputed" {
		return "disputed"
	}
	if previous == "verified" && checked {
		return "verified"
	}
	return "reported"
}
