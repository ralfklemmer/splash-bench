package check

// Recommendation says how far a target model fits into the Mac's memory.
type Recommendation struct {
	Model TargetModel
	// Tier is the largest fitting index into ContextTiers, or -1 when not
	// even the smallest context fits.
	Tier int
}

// Fits reports whether the model runs at all.
func (r Recommendation) Fits() bool {
	return r.Tier >= 0
}

// Recommend picks, per catalog model, the largest context tier whose
// memory estimate fits into the usable memory.
func Recommend(mem Memory, catalog []TargetModel) []Recommendation {
	budget := mem.UsableGB()
	recs := make([]Recommendation, 0, len(catalog))
	for _, model := range catalog {
		tier := -1
		for i := range ContextTiers {
			if model.MemoryGB(i) <= budget {
				tier = i
			}
		}
		recs = append(recs, Recommendation{Model: model, Tier: tier})
	}
	return recs
}
