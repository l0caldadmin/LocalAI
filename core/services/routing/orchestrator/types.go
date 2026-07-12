package orchestrator

import (
	"time"

	"github.com/mudler/LocalAI/core/config"
	"github.com/mudler/LocalAI/core/services/routing/admission"
)

// OutcomeCode defines the explicit taxonomy of routing outcomes.
type OutcomeCode string

const (
	OutcomeSuccess                   OutcomeCode = "SUCCESS"
	OutcomeError                     OutcomeCode = "ERROR"
	OutcomeNoRouteNoCandidates       OutcomeCode = "NO_ROUTE_NO_CANDIDATES"
	OutcomeNoRoutePolicyDenied       OutcomeCode = "NO_ROUTE_POLICY_DENIED"
	OutcomeNoRouteCapabilityExceeded OutcomeCode = "NO_ROUTE_CAPABILITY_EXCEEDED"
	OutcomeFallbackExhausted         OutcomeCode = "FALLBACK_EXHAUSTED"
	OutcomeCircuitOpen               OutcomeCode = "CIRCUIT_OPEN"
	OutcomeMidStreamFailure          OutcomeCode = "MID_STREAM_FAILURE"
)

// FallbackAttempt records a single candidate execution attempt within an ExecutionTrace.
type FallbackAttempt struct {
	Model    string        `json:"model"`
	NodeID   string        `json:"node_id,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
	Status   OutcomeCode   `json:"status"`
	Reason   string        `json:"reason,omitempty"`
}

// TrustBoundary represents the exact transition of trust and network scope when falling back.
type TrustBoundary struct {
	FromTrust config.TrustClass   `json:"from_trust"`
	ToTrust   config.TrustClass   `json:"to_trust"`
	FromScope config.NetworkScope `json:"from_scope"`
	ToScope   config.NetworkScope `json:"to_scope"`
}

// ProvenanceStep explicitly models an atomic action in the fallback chain.
type ProvenanceStep struct {
	Model             string                      `json:"model"`
	AdmissionDecision admission.AdmissionDecision `json:"admission_decision"`
	ExecutionAttempt  *FallbackAttempt            `json:"execution_attempt,omitempty"`
	BoundaryCrossed   *TrustBoundary              `json:"boundary_crossed,omitempty"`
}

// ProvenanceChain acts as an append-only ledger for the request lifecycle,
// substituting the older ExecutionTrace.
type ProvenanceChain struct {
	// Initial Context
	RequestPolicy admission.RequestPolicy `json:"request_policy"`
	RoutePlan     []string                `json:"route_plan"`

	// The Step-by-Step Ledger (Append-Only)
	Steps []ProvenanceStep `json:"steps"`

	// Terminal Outcome
	FinalTrustPath string      `json:"final_trust_path,omitempty"`
	FinalOutcome   OutcomeCode `json:"final_outcome,omitempty"`
}
