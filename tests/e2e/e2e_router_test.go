package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Router e2e: drives /v1/chat/completions through the RouteModel middleware
// against a configured score classifier (mock-classifier from the suite
// fixtures) and two candidates. The mock-backend's Score handler ranks
// candidates by looking for a `ROUTE_HINT=<label>` marker in the prompt and
// boosting the candidate whose label matches; without a hint, all candidates
// score equally and the router falls back. The ECHO_SERVED_MODEL trigger
// makes the chosen candidate echo its loaded model file path so the test can
// verify routing decisively rather than infer it from content shape.
var _ = Describe("Router E2E", Label("Router"), func() {
	chat := func(message string) (*openai.ChatCompletion, error) {
		return client.Chat.Completions.New(
			context.TODO(),
			openai.ChatCompletionNewParams{
				Model: "smart-router",
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage(message),
				},
			},
		)
	}

	It("routes a casual probe to the casual-chat candidate", func() {
		resp, err := chat("ROUTE_HINT=casual-chat ECHO_SERVED_MODEL")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Choices).To(HaveLen(1))
		Expect(resp.Choices[0].Message.Content).To(ContainSubstring("SERVED_MODEL=mock-cand-casual.bin"),
			"casual hint should have routed to mock-cand-casual; got %q", resp.Choices[0].Message.Content)

		verifyExecutionTrace("smart-router", "mock-cand-casual")
	})

	It("routes a code probe to the code-generation candidate", func() {
		resp, err := chat("ROUTE_HINT=code-generation ECHO_SERVED_MODEL")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Choices).To(HaveLen(1))
		Expect(resp.Choices[0].Message.Content).To(ContainSubstring("SERVED_MODEL=mock-cand-code.bin"),
			"code hint should have routed to mock-cand-code; got %q", resp.Choices[0].Message.Content)
	})

	It("falls back when no policy label matches the probe", func() {
		// No ROUTE_HINT marker — the mock Score handler gives every candidate
		// the same base log-prob, softmax goes uniform, no label clears
		// activation_threshold=0.40, so the router falls back to
		// mock-cand-casual.
		resp, err := chat("ECHO_SERVED_MODEL hello world")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Choices).To(HaveLen(1))
		Expect(resp.Choices[0].Message.Content).To(ContainSubstring("SERVED_MODEL=mock-cand-casual.bin"),
			"unhinted probe should have fallen back; got %q", resp.Choices[0].Message.Content)
	})

	It("falls back to secondary candidate when primary fails during execution", func() {
		// We send ROUTE_HINT=casual-chat to force routing to mock-cand-casual.
		// We also send MOCK_ERROR=mock-cand-casual.bin, which causes the mock backend Predict to return an error for only that model.
		// Since mock-cand-casual has a fallback to mock-cand-fallback, the orchestrator
		// should catch the error and execute mock-cand-fallback instead.
		resp, err := chat("ROUTE_HINT=casual-chat MOCK_ERROR=mock-cand-casual.bin ECHO_SERVED_MODEL")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Choices).To(HaveLen(1))
		Expect(resp.Choices[0].Message.Content).To(ContainSubstring("SERVED_MODEL=mock-cand-fallback.bin"),
			"failed primary candidate should have fallen back to secondary; got %q", resp.Choices[0].Message.Content)

		// Verify telemetry trace has the failure attempt and the success attempt
		// adminURL for trace
		adminURL := strings.Replace(apiURL, "/v1", "/api/router/decisions?limit=1", 1)
		adminResp, err := http.Get(adminURL)
		Expect(err).ToNot(HaveOccurred())
		defer adminResp.Body.Close()
		Expect(adminResp.StatusCode).To(Equal(http.StatusOK))

		body, err := io.ReadAll(adminResp.Body)
		Expect(err).ToNot(HaveOccurred())

		var data struct {
			Decisions []struct {
				ServedModel string `json:"served_model"`
				Trace       struct {
					FinalOutcome string `json:"final_outcome"`
					Steps     []struct {
						Model  string `json:"model"`
						ExecutionAttempt *struct {
							Model  string `json:"model"`
							Status string `json:"status"`
						} `json:"execution_attempt,omitempty"`
					} `json:"steps"`
				} `json:"trace"`
			} `json:"decisions"`
		}
		Expect(json.Unmarshal(body, &data)).To(Succeed())
		Expect(data.Decisions).ToNot(BeEmpty())

		latest := data.Decisions[0]
		Expect(latest.ServedModel).To(Equal("mock-cand-casual"))
		Expect(latest.Trace.FinalOutcome).To(Equal("SUCCESS"))
		Expect(latest.Trace.Steps).To(HaveLen(2))
		
		Expect(latest.Trace.Steps[0].Model).To(Equal("mock-cand-casual"))
		Expect(latest.Trace.Steps[0].ExecutionAttempt.Status).To(Equal("ERROR"))
		
		Expect(latest.Trace.Steps[1].Model).To(Equal("mock-cand-fallback"))
		Expect(latest.Trace.Steps[1].ExecutionAttempt.Status).To(Equal("SUCCESS"))
	})

	It("routes correctly over a long conversation (exercises fitMessages)", func() {
		// Build a conversation long enough that the score classifier's
		// probeTokenBudget kicks in and fitMessages has to trim. mock-backend's
		// TokenizeString returns ~1 token per 4 prompt characters, and the
		// classifier ContextSize is 4096, so >40k chars guarantees the trim
		// path. The ROUTE_HINT marker is placed ONLY in the newest message —
		// if fitMessages dropped it during trim, no candidate would win and we
		// would route to the fallback (mock-cand-casual) instead of the code
		// candidate.
		filler := strings.Repeat("background context, lorem ipsum dolor sit amet. ", 200) // ~10k chars × 5 turns
		msgs := make([]openai.ChatCompletionMessageParamUnion, 0, 6)
		for range 5 {
			msgs = append(msgs, openai.UserMessage(filler))
		}
		msgs = append(msgs, openai.UserMessage("ROUTE_HINT=code-generation ECHO_SERVED_MODEL"))

		resp, err := client.Chat.Completions.New(
			context.TODO(),
			openai.ChatCompletionNewParams{Model: "smart-router", Messages: msgs},
		)
		Expect(err).ToNot(HaveOccurred(), "router must survive a long conversation without erroring")
		Expect(resp.Choices).To(HaveLen(1))
		// The newest turn carries the routing intent ("code"); fitMessages must
		// keep it intact even after dropping older fillers, so the code
		// candidate still wins.
		Expect(resp.Choices[0].Message.Content).To(ContainSubstring("SERVED_MODEL=mock-cand-code.bin"),
			"long-conversation routing must still resolve to the code candidate; got %q",
			resp.Choices[0].Message.Content)
	})

	It("Zero-Execution Provenance: drops untrusted candidates pre-flight and denies untrusted at admission", func() {
		// We set X-LocalAI-Require-Trust: 100 via openai client headers.
		// mock-cand-untrusted is TrustUntrusted (10). It should be denied.
		// The prompt contains MOCK_CRASH=mock-cand-untrusted to prove zero execution.
		// Wait, the client is global, so we need to instantiate a local client with headers.
		customClient := openai.NewClient(
			option.WithBaseURL(apiURL),
			option.WithHeader("X-LocalAI-Require-Trust", "100"),
		)
		
		// We route to 'smart-router-strict' which has mock-cand-untrusted as a fallback.
		// Wait, we can just use smart-router but ensure mock-cand-untrusted is in the plan.
		// Let's just use the default router but force mock-cand-untrusted using ROUTE_HINT.
		_, err := customClient.Chat.Completions.New(
			context.TODO(),
			openai.ChatCompletionNewParams{
				Model: "smart-router",
				Messages: []openai.ChatCompletionMessageParamUnion{
					openai.UserMessage("ROUTE_HINT=untrusted MOCK_CRASH=mock-cand-untrusted ECHO_SERVED_MODEL"),
				},
			},
		)
		
		// It should fail to route entirely because the only candidate matched is denied, 
		// and the router fallback chain is exhausted.
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("policy denied")) // Or however the 500 error surfaces

		// Verify telemetry trace has the denial
		adminURL := strings.Replace(apiURL, "/v1", "/api/router/decisions?limit=1", 1)
		adminResp, err := http.Get(adminURL)
		Expect(err).ToNot(HaveOccurred())
		defer adminResp.Body.Close()
		Expect(adminResp.StatusCode).To(Equal(http.StatusOK))

		body, err := io.ReadAll(adminResp.Body)
		Expect(err).ToNot(HaveOccurred())

		var data struct {
			Decisions []struct {
				ServedModel string `json:"served_model"`
				Trace       struct {
					FinalOutcome string `json:"final_outcome"`
					Steps     []struct {
						Model  string `json:"model"`
						AdmissionDecision struct {
							Allowed bool `json:"allowed"`
							Reason  string `json:"reason"`
						} `json:"admission_decision"`
						ExecutionAttempt *struct {
							Model  string `json:"model"`
							Status string `json:"status"`
						} `json:"execution_attempt,omitempty"`
					} `json:"steps"`
				} `json:"trace"`
			} `json:"decisions"`
		}
		Expect(json.Unmarshal(body, &data)).To(Succeed())
		Expect(data.Decisions).ToNot(BeEmpty())

		latest := data.Decisions[0]
		Expect(latest.Trace.FinalOutcome).To(Equal("FALLBACK_EXHAUSTED"))
		Expect(latest.Trace.Steps).ToNot(BeEmpty())
		
		// The step for mock-cand-untrusted should show Allowed: false
		var found bool
		for _, step := range latest.Trace.Steps {
			if step.Model == "mock-cand-untrusted" {
				found = true
				Expect(step.AdmissionDecision.Allowed).To(BeFalse())
				Expect(step.AdmissionDecision.Reason).To(ContainSubstring("candidate trust class is too low"))
				Expect(step.ExecutionAttempt.Status).To(Equal("NO_ROUTE_POLICY_DENIED"))
			}
		}
		Expect(found).To(BeTrue(), "mock-cand-untrusted must appear in the trace as denied")
	})
})

func verifyExecutionTrace(routerModel, expectedCandidate string) {
	// apiURL is typically "http://127.0.0.1:%d/v1", we need "http://127.0.0.1:%d/api/router/decisions"
	adminURL := strings.Replace(apiURL, "/v1", "/api/router/decisions?limit=1", 1)
	resp, err := http.Get(adminURL)
	Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	body, err := io.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred())

	var data struct {
		Decisions []struct {
			RouterModel string `json:"router_model"`
			ServedModel string `json:"served_model"`
			Trace       struct {
				FinalOutcome string `json:"final_outcome"`
				Steps     []struct {
					Model  string `json:"model"`
					ExecutionAttempt *struct {
						Model  string `json:"model"`
						Status string `json:"status"`
					} `json:"execution_attempt,omitempty"`
				} `json:"steps"`
			} `json:"trace"`
		} `json:"decisions"`
	}
	Expect(json.Unmarshal(body, &data)).To(Succeed())
	Expect(data.Decisions).ToNot(BeEmpty(), "should have at least one decision record")

	latest := data.Decisions[0]
	Expect(latest.RouterModel).To(Equal(routerModel))
	Expect(latest.ServedModel).To(Equal(expectedCandidate))

	// Verify Phase 1 telemetry envelope
	Expect(latest.Trace.FinalOutcome).To(Equal("SUCCESS"))
	Expect(latest.Trace.Steps).To(HaveLen(1))
	Expect(latest.Trace.Steps[0].Model).To(Equal(expectedCandidate))
	Expect(latest.Trace.Steps[0].ExecutionAttempt.Status).To(Equal("SUCCESS"))
}
