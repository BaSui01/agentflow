package runtimepolicy

import (
	"context"
	"errors"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
)

// MultiUsageRecorder fans out a usage record to multiple recorders.
type MultiUsageRecorder struct {
	Recorders []router.UsageRecorder
}

// RecordUsage implements router.UsageRecorder.
func (m MultiUsageRecorder) RecordUsage(ctx context.Context, usage *router.ChannelUsageRecord) error {
	var errs []error
	for _, recorder := range m.Recorders {
		if recorder == nil {
			continue
		}
		if err := recorder.RecordUsage(ctx, cloneUsageRecord(usage)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// MultiCooldownController fans out cooldown checks and writebacks to multiple controllers.
type MultiCooldownController struct {
	Controllers []router.CooldownController
}

// Allow implements router.CooldownController.
func (m MultiCooldownController) Allow(ctx context.Context, request *router.ChannelRouteRequest, selection *router.ChannelSelection) error {
	var errs []error
	for _, controller := range m.Controllers {
		if controller == nil {
			continue
		}
		if err := controller.Allow(ctx, cloneRouteRequest(request), cloneSelection(selection)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// RecordResult implements router.CooldownController.
func (m MultiCooldownController) RecordResult(ctx context.Context, usage *router.ChannelUsageRecord) error {
	var errs []error
	for _, controller := range m.Controllers {
		if controller == nil {
			continue
		}
		if err := controller.RecordResult(ctx, cloneUsageRecord(usage)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// MultiQuotaPolicy fans out quota checks and writebacks to multiple policies.
type MultiQuotaPolicy struct {
	Policies []router.QuotaPolicy
}

// Allow implements router.QuotaPolicy.
func (m MultiQuotaPolicy) Allow(ctx context.Context, request *router.ChannelRouteRequest, selection *router.ChannelSelection) error {
	var errs []error
	for _, policy := range m.Policies {
		if policy == nil {
			continue
		}
		if err := policy.Allow(ctx, cloneRouteRequest(request), cloneSelection(selection)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// RecordUsage implements router.QuotaPolicy.
func (m MultiQuotaPolicy) RecordUsage(ctx context.Context, usage *router.ChannelUsageRecord) error {
	var errs []error
	for _, policy := range m.Policies {
		if policy == nil {
			continue
		}
		if err := policy.RecordUsage(ctx, cloneUsageRecord(usage)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func cloneRouteRequest(request *router.ChannelRouteRequest) *router.ChannelRouteRequest {
	if request == nil {
		return nil
	}
	cloned := *request
	cloned.Metadata = cloneStringMap(request.Metadata)
	cloned.Tags = cloneStrings(request.Tags)
	cloned.ExcludedChannelIDs = cloneStrings(request.ExcludedChannelIDs)
	cloned.ExcludedKeyIDs = cloneStrings(request.ExcludedKeyIDs)
	return &cloned
}

func cloneSelection(selection *router.ChannelSelection) *router.ChannelSelection {
	if selection == nil {
		return nil
	}
	cloned := *selection
	cloned.Metadata = cloneStringMap(selection.Metadata)
	return &cloned
}

func cloneUsageRecord(record *router.ChannelUsageRecord) *router.ChannelUsageRecord {
	if record == nil {
		return nil
	}
	cloned := *record
	if record.Usage != nil {
		usage := *record.Usage
		cloned.Usage = &usage
	}
	cloned.Metadata = cloneStringMap(record.Metadata)
	return &cloned
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
