package registry

import (
	"github.com/mudler/LocalAI/core/config"
)

// ExtractMetadata takes a candidate's model configuration and returns its typed
// CandidateMetadata, applying strict defaults if properties are missing or unknown.
func ExtractMetadata(cfg *config.ModelConfig) config.CandidateMetadata {
	// Start with the raw metadata declared in YAML
	meta := cfg.Metadata

	// 1. Resolve Execution Mode (Default to Local if entirely absent)
	if meta.ExecutionMode == config.ExecModeUnknown {
		meta.ExecutionMode = config.ExecModeLocal
	}

	// 2. Apply Mode-Specific Fail-Open / Fail-Closed Defaults
	switch meta.ExecutionMode {
	case config.ExecModeLocal:
		// Local models are permissive by default
		if meta.TrustClass == config.TrustUnknown {
			meta.TrustClass = config.TrustStandard
		}
		if meta.PrivacyClass == config.PrivacyUnknown {
			meta.PrivacyClass = config.PrivacyInternal
		}
		if meta.NetworkScope == config.NetScopeUnknown {
			meta.NetworkScope = config.NetScopeLocalhost
		}
	default:
		// Federated/Remote models fail closed by default (zero-trust, fully public)
		if meta.TrustClass == config.TrustUnknown {
			meta.TrustClass = config.TrustUntrusted
		}
		if meta.PrivacyClass == config.PrivacyUnknown {
			meta.PrivacyClass = config.PrivacyPublic
		}
		if meta.NetworkScope == config.NetScopeUnknown {
			meta.NetworkScope = config.NetScopeInternet
		}
	}

	return meta
}
