package jsonutil_test

import (
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/pkg/jsonutil"
)

func TestUnwrapStringifiedJSONRawMessage(t *testing.T) {
	raw := json.RawMessage(`"{\"city\":\"北京\"}"`)

	got := jsonutil.UnwrapStringifiedRawMessage(raw)

	if string(got) != `{"city":"北京"}` {
		t.Fatalf("unexpected unwrapped json: %s", got)
	}
}

func TestUnwrapStringifiedJSONRawMessagePreservesNormalAndInvalidInput(t *testing.T) {
	cases := []json.RawMessage{
		json.RawMessage(`{"city":"北京"}`),
		json.RawMessage(`[1,2]`),
		json.RawMessage(`"not json"`),
		json.RawMessage(`{bad`),
	}
	for _, tc := range cases {
		got := jsonutil.UnwrapStringifiedRawMessage(tc)
		if string(got) != string(tc) {
			t.Fatalf("input %s should be preserved, got %s", tc, got)
		}
	}
}
