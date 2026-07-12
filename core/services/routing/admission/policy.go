package admission

import (
	"fmt"

	"github.com/mudler/LocalAI/core/config"
)

// RequestPolicy represents the request-level boundaries asserted by the tenant,
// authentication middleware, or explicit client headers.
type RequestPolicy struct {
	RequiredPrivacy config.PrivacyClass
	RequiredTrust   config.TrustClass
	FederationDeny  bool
	RequiredCaps    []config.CapabilityTag
}

// AdmissionDecision is the structured result of evaluating a candidate against a policy.
type AdmissionDecision struct {
	Allowed             bool
	Reason              string
	MatchedPolicy       string
	MissingRequirements []string
	CandidateFacts      config.CandidateMetadata
}

// EvaluatePolicy compares the facts of a candidate model against the strict requirements
// of the request policy. It fails closed on any unmet requirement.
func EvaluatePolicy(req RequestPolicy, cand config.CandidateMetadata) AdmissionDecision {
	decision := AdmissionDecision{
		Allowed:        true,
		CandidateFacts: cand,
	}
	var missing []string

	// 1. Evaluate Federation Denial
	if req.FederationDeny && cand.ExecutionMode == config.ExecModeFederated {
		decision.Allowed = false
		missing = append(missing, "execution_mode:!federated")
		decision.Reason = "federated execution is explicitly denied by request policy"
	}

	// 2. Evaluate Trust Class (Integer comparison: Candidate must be >= Required)
	if cand.TrustClass < req.RequiredTrust {
		decision.Allowed = false
		missing = append(missing, fmt.Sprintf("trust>=%d", req.RequiredTrust))
		if decision.Reason == "" {
			decision.Reason = "candidate trust class is too low"
		}
	}

	// 3. Evaluate Privacy Class
	if cand.PrivacyClass < req.RequiredPrivacy {
		decision.Allowed = false
		missing = append(missing, fmt.Sprintf("privacy>=%d", req.RequiredPrivacy))
		if decision.Reason == "" {
			decision.Reason = "candidate privacy class is too low"
		}
	}

	// 4. Evaluate Capabilities (Candidate must have ALL required caps)
	for _, reqCap := range req.RequiredCaps {
		hasCap := false
		for _, candCap := range cand.Capabilities {
			if candCap == reqCap {
				hasCap = true
				break
			}
		}
		if !hasCap {
			decision.Allowed = false
			missing = append(missing, fmt.Sprintf("cap:%s", reqCap))
			if decision.Reason == "" {
				decision.Reason = "candidate missing required capability"
			}
		}
	}

	if !decision.Allowed {
		decision.MissingRequirements = missing
		decision.MatchedPolicy = "RequestPolicy" // For v1, this is the hardcoded policy source
	}

	return decision
}
