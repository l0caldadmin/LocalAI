package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/mudler/LocalAI/core/config"
	"github.com/mudler/LocalAI/core/services/routing/admission"
	"github.com/mudler/LocalAI/core/services/routing/registry"
	"github.com/mudler/xlog"
)

// CandidateLoader loads a candidate config by name.
type CandidateLoader func(name string) (*config.ModelConfig, error)

// ExecuteWithPlan iterates over the configured fallbacks of a model.
// The provided execution closure must return true for `committed` if it has
// polluted the response (e.g. sent HTTP streaming headers) such that a
// clean fallback is no longer possible.
func ExecuteWithPlan(
	ctx context.Context,
	reqPolicy admission.RequestPolicy,
	cfg *config.ModelConfig,
	trace *ProvenanceChain,
	loader CandidateLoader,
	execute func(candCfg *config.ModelConfig) (bool, error),
) error {
	if trace == nil {
		trace = &ProvenanceChain{} // Fallback safety
	}

	trace.FinalOutcome = OutcomeError
	trace.RequestPolicy = reqPolicy
	trace.Steps = []ProvenanceStep{}

	rawCandidates := []string{cfg.Name}
	rawCandidates = append(rawCandidates, cfg.Router.Fallbacks...)

	type candEntry struct {
		name string
		meta config.CandidateMetadata
	}
	var preflight []candEntry
	for _, name := range rawCandidates {
		candCfg, err := loader(name)
		if err == nil {
			preflight = append(preflight, candEntry{
				name: name,
				meta: registry.ExtractMetadata(candCfg),
			})
		} else {
			// If it doesn't load, keep it so it fails normally in the loop
			preflight = append(preflight, candEntry{
				name: name,
			})
		}
	}

	// Sort by TrustClass descending (Highest trust first)
	// Stable sort preserves original order for equal trust
	for i := 0; i < len(preflight); i++ {
		for j := i + 1; j < len(preflight); j++ {
			if preflight[j].meta.TrustClass > preflight[i].meta.TrustClass {
				preflight[i], preflight[j] = preflight[j], preflight[i]
			}
		}
	}

	// Pre-flight truncation for Confidential privacy
	var finalCandidates []string
	for _, c := range preflight {
		if reqPolicy.RequiredPrivacy == config.PrivacyConfidential {
			if c.meta.TrustClass < config.TrustHigh {
				xlog.Debug("ExecuteWithPlan dropping candidate due to mixed-trust pre-flight check", "candidate", c.name)
				continue
			}
		}
		finalCandidates = append(finalCandidates, c.name)
	}

	trace.RoutePlan = finalCandidates
	candidates := finalCandidates

	xlog.Debug("ExecuteWithPlan started", "candidates", candidates)

	var lastErr error

	for _, candName := range candidates {
		xlog.Debug("ExecuteWithPlan trying candidate", "candidate", candName)
		start := time.Now()

		candCfg, err := loader(candName)
		if err != nil {
			// Atomic commit for load failure
			trace.Steps = append(trace.Steps, ProvenanceStep{
				Model: candName,
				ExecutionAttempt: &FallbackAttempt{
					Model:    candName,
					Status:   OutcomeError,
					Reason:   err.Error(),
					Duration: time.Since(start),
				},
			})
			lastErr = err
			continue
		}

		// 1. Extract Typed Facts
		candMeta := registry.ExtractMetadata(candCfg)

		// 2. Evaluate Policy
		decision := admission.EvaluatePolicy(reqPolicy, candMeta)
		if !decision.Allowed {
			// Atomic commit for policy denial
			trace.Steps = append(trace.Steps, ProvenanceStep{
				Model:             candName,
				AdmissionDecision: decision,
				ExecutionAttempt: &FallbackAttempt{
					Model:  candName,
					Status: OutcomeNoRoutePolicyDenied,
					Reason: decision.Reason,
				},
			})
			lastErr = fmt.Errorf("policy denied: %s", decision.Reason)
			continue
		}

		// 3. Execution (Admitted)
		committed, err := execute(candCfg)
		duration := time.Since(start)

		if err == nil {
			// Atomic commit for success
			trace.Steps = append(trace.Steps, ProvenanceStep{
				Model:             candName,
				AdmissionDecision: decision,
				ExecutionAttempt: &FallbackAttempt{
					Model:    candName,
					Status:   OutcomeSuccess,
					Duration: duration,
				},
				BoundaryCrossed: &TrustBoundary{
					FromTrust: reqPolicy.RequiredTrust,
					ToTrust:   candMeta.TrustClass,
					FromScope: "", // To be populated properly
					ToScope:   candMeta.NetworkScope,
				},
			})
			trace.FinalOutcome = OutcomeSuccess
			trace.FinalTrustPath = fmt.Sprintf("%s:%d", candMeta.ExecutionMode, candMeta.TrustClass)
			return nil
		}

		xlog.Debug("ExecuteWithPlan attempt finished", "candidate", candName, "committed", committed, "error", err)

		// Atomic commit for execution failure
		trace.Steps = append(trace.Steps, ProvenanceStep{
			Model:             candName,
			AdmissionDecision: decision,
			ExecutionAttempt: &FallbackAttempt{
				Model:    candName,
				Status:   OutcomeError,
				Reason:   err.Error(),
				Duration: duration,
			},
		})
		lastErr = err

		// If the handler committed the response (e.g. wrote headers),
		// it is no longer safe to fallback to another candidate.
		if committed {
			xlog.Debug("ExecuteWithPlan response committed, breaking fallback loop")
			trace.FinalOutcome = OutcomeMidStreamFailure
			break
		}
	}

	xlog.Debug("ExecuteWithPlan complete", "finalErr", lastErr)
	if trace.FinalOutcome == OutcomeError {
		trace.FinalOutcome = OutcomeFallbackExhausted
	}
	return lastErr
}
