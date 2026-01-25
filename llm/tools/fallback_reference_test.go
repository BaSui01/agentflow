package tools

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestToolCallChain_ResolveReferences(t *testing.T) {
	chain := &ToolCallChain{}
	args := json.RawMessage(`{
		"city": "${call_1.location}",
		"temp": "${call_1.temperature}",
		"msg": "city=${call_1.location}",
		"missing": "${call_2.x}",
		"payload": "${call_1}"
	}`)
	ctx := map[string]json.RawMessage{
		"call_1": json.RawMessage(`{"location":"Beijing","temperature":22,"meta":{"k":"v"}}`),
	}

	out := chain.resolveReferences(args, ctx)

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal output failed: %v", err)
	}

	if got["city"] != "Beijing" {
		t.Fatalf("city mismatch: %v", got["city"])
	}
	if got["temp"] != float64(22) {
		t.Fatalf("temp mismatch: %v", got["temp"])
	}
	if got["msg"] != "city=Beijing" {
		t.Fatalf("msg mismatch: %v", got["msg"])
	}
	if got["missing"] != "${call_2.x}" {
		t.Fatalf("missing should stay unchanged: %v", got["missing"])
	}

	wantPayload := map[string]any{
		"location":    "Beijing",
		"temperature": float64(22),
		"meta": map[string]any{
			"k": "v",
		},
	}
	if !reflect.DeepEqual(got["payload"], wantPayload) {
		t.Fatalf("payload mismatch: got=%v want=%v", got["payload"], wantPayload)
	}
}
