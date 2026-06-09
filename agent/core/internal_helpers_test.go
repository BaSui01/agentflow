package core

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type timelineRecorderStub struct{}

func (timelineRecorderStub) StartTrace(string, string)                                        {}
func (timelineRecorderStub) EndTrace(string, string, error)                                   {}
func (timelineRecorderStub) RecordTask(string, bool, time.Duration, int, float64, float64)    {}
func (timelineRecorderStub) AddExplainabilityTimeline(string, string, string, map[string]any) {}

func TestNormalizeInstructionListTrimsDeduplicatesAndDropsEmpty(t *testing.T) {
	got := NormalizeInstructionList([]string{" build ", "", "test", "build", "  deploy  "})
	assert.Equal(t, []string{"build", "test", "deploy"}, got)
	assert.Nil(t, NormalizeInstructionList([]string{" ", ""}))
	assert.Nil(t, NormalizeInstructionList(nil))
}

func TestExplainabilityTimelineRecorderFromNarrowsOptionalCapability(t *testing.T) {
	recorder := &timelineRecorderStub{}
	assert.Same(t, recorder, ExplainabilityTimelineRecorderFrom(recorder))

	var obs ObservabilityRunner
	assert.Nil(t, ExplainabilityTimelineRecorderFrom(obs))
}

func TestAppendUniqueStringIsCaseInsensitive(t *testing.T) {
	values := []string{"Build"}
	values = AppendUniqueString(values, " build ")
	values = AppendUniqueString(values, "test")
	values = AppendUniqueString(values, " ")

	assert.Equal(t, []string{"Build", "test"}, values)
}

func TestFallbackStringReturnsFirstTrimmedValue(t *testing.T) {
	assert.Equal(t, "value", FallbackString("", "  ", " value ", "later"))
	assert.Empty(t, FallbackString("", "  "))
}

func TestPanicPayloadToErrorPreservesErrors(t *testing.T) {
	boom := errors.New("boom")
	assert.Same(t, boom, PanicPayloadToError(boom))
	assert.EqualError(t, PanicPayloadToError("boom"), "panic: boom")
}
