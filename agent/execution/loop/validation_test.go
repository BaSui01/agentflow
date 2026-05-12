package loop

import "testing"

type warningProviderStub struct{ warnings []string }

func (w warningProviderStub) Validate(CodeValidationLanguage, string) []string { return w.warnings }

func TestValidateGenericCoversAcceptanceAndGoalValidation(t *testing.T) {
	result := ValidateGeneric(
		&Input{Context: map[string]any{"acceptance_criteria": []any{"tests pass", " docs updated "}}},
		&State{Goal: "verify implementation"},
		&Output{Content: "done", Metadata: map[string]any{"acceptance_criteria_met": false}},
		nil,
	)

	if result.Status != ValidationStatusPending || !result.Pending || result.Passed {
		t.Fatalf("unexpected status: %#v", result)
	}
	if result.Reason != "acceptance criteria not met" {
		t.Fatalf("reason = %q", result.Reason)
	}
	assertStringSlice(t, result.AcceptanceCriteria, []string{"tests pass", "docs updated"})
	assertStringSlice(t, result.UnresolvedItems, []string{"validate acceptance criteria"})
	if got := result.Metadata["acceptance_criteria_met"]; got != false {
		t.Fatalf("metadata acceptance_criteria_met = %#v", got)
	}
}

func TestValidateToolVerificationRequiredByMetadata(t *testing.T) {
	pending := ValidateToolVerification(nil, nil, &Output{Content: "answer", Metadata: map[string]any{"tool_used": true}}, nil)
	if pending.Status != ValidationStatusPending || pending.Reason != "tool verification pending" {
		t.Fatalf("unexpected pending validation: %#v", pending)
	}
	assertStringSlice(t, pending.UnresolvedItems, []string{"verify tool-backed output"})

	failed := ValidateToolVerification(nil, nil, &Output{Metadata: map[string]any{"tool_verification_required": true, "verified": false}}, nil)
	if failed.Status != ValidationStatusFailed || failed.Reason != "tool verification failed" {
		t.Fatalf("unexpected failed validation: %#v", failed)
	}
}

func TestValidateCodeTaskDetectsMissingEvidenceAndWarnings(t *testing.T) {
	missing := ValidateCodeTask(nil, &Input{Context: map[string]any{"task_type": "bugfix"}}, nil, &Output{Content: "patch"}, nil)
	if missing.Status != ValidationStatusPending || missing.Reason != "code task requires tests or verification evidence" {
		t.Fatalf("unexpected missing validation: %#v", missing)
	}
	assertStringSlice(t, missing.UnresolvedItems, []string{"run tests or verification for code changes"})

	warned := ValidateCodeTask(
		warningProviderStub{warnings: []string{"unsafe shell"}},
		&Input{Context: map[string]any{"requires_code": true}},
		nil,
		&Output{Metadata: map[string]any{"tests_passed": true, "code_language": "go", "generated_code": "package main"}},
		nil,
	)
	if warned.Status != ValidationStatusPending {
		t.Fatalf("expected warnings to keep validation pending: %#v", warned)
	}
	assertStringSlice(t, warned.RemainingRisks, []string{"unsafe shell"})
}

func TestMergeValidationResultKeepsWorstStatusAndMetadata(t *testing.T) {
	target := NewValidationResult(ValidationStatusPassed, "ok")
	target.UnresolvedItems = []string{"old"}
	FinalizeValidationResult(target)
	incoming := &ValidationResult{
		Status:          ValidationStatusFailed,
		Reason:          "broken",
		UnresolvedItems: []string{"old", "new"},
		RemainingRisks:  []string{"risk"},
		Issues:          []ValidationIssue{{Validator: "generic", Code: "failed"}},
		Metadata:        map[string]any{"source": "incoming"},
	}

	MergeValidationResult(target, incoming)

	if target.Status != ValidationStatusFailed || target.Reason != "broken" {
		t.Fatalf("unexpected merged result: %#v", target)
	}
	assertStringSlice(t, target.UnresolvedItems, []string{"old", "new"})
	assertStringSlice(t, target.RemainingRisks, []string{"risk"})
	if len(target.Issues) != 1 || target.Metadata["source"] != "incoming" {
		t.Fatalf("issues/metadata not merged: %#v", target)
	}
}

func TestCodeSnippetForValidationAliases(t *testing.T) {
	tests := []struct {
		lang string
		want CodeValidationLanguage
	}{
		{lang: "js", want: CodeLangJavaScript},
		{lang: "ts", want: CodeLangTypeScript},
		{lang: "golang", want: CodeLangGo},
		{lang: "shell", want: CodeLangBash},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			lang, code, ok := CodeSnippetForValidation(&Output{Metadata: map[string]any{"language": tt.lang, "code": "echo ok"}})
			if !ok || lang != tt.want || code != "echo ok" {
				t.Fatalf("snippet = (%q,%q,%v), want (%q,echo ok,true)", lang, code, ok, tt.want)
			}
		})
	}
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slice len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slice[%d] = %q, want %q; full=%#v", i, got[i], want[i], got)
		}
	}
}
