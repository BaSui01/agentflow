package agent

// ReasoningExposureLevel controls which non-default reasoning patterns are
// registered into the runtime. The official execution path remains react,
// with reflection as an opt-in quality enhancement outside the registry.
type ReasoningExposureLevel string

const (
	ReasoningExposureOfficial ReasoningExposureLevel = "official"
	ReasoningExposureAdvanced ReasoningExposureLevel = "advanced"
	ReasoningExposureAll      ReasoningExposureLevel = "all"
)

func normalizeReasoningExposureLevel(level ReasoningExposureLevel) ReasoningExposureLevel {
	switch level {
	case ReasoningExposureAdvanced, ReasoningExposureAll:
		return level
	default:
		return ReasoningExposureOfficial
	}
}
