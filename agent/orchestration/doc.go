// Package orchestration provides a unified orchestrator that dynamically selects
// between collaboration, crews, hierarchical, and handoff patterns based on task
// characteristics and agent composition.
//
// Stability: Beta
//
// The orchestrator analyzes the provided agents and task metadata to automatically
// choose the most appropriate multi-agent coordination pattern, or allows explicit
// pattern selection via configuration.
package orchestration
