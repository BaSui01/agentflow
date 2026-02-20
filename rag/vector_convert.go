package rag

// Float32ToFloat64 converts a []float32 vector to []float64.
// Useful when integrating external systems that produce float32 embeddings
// with AgentFlow's float64-based VectorStore and GraphVectorStore interfaces.
func Float32ToFloat64(v []float32) []float64 {
	if v == nil {
		return nil
	}
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return out
}

// Float64ToFloat32 converts a []float64 vector to []float32.
// Useful when sending embeddings to external systems that require float32 precision.
func Float64ToFloat32(v []float64) []float32 {
	if v == nil {
		return nil
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(x)
	}
	return out
}
