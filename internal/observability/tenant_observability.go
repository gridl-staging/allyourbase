// Package observability provides tenant-scoped tracing attributes and quota metrics for observing tenant resource usage and quota violations.
package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	TenantIDKey     = "tenant_id"
	TenantNameKey   = "tenant_name"
	ResourceKey     = "resource"
	CurrentKey      = "current"
	LimitKey        = "limit"
	OperationKey    = "operation"
	TargetTenantKey = "target_tenant_id"
)

func TenantIDAttr(id string) attribute.KeyValue {
	return attribute.String(TenantIDKey, id)
}

func TenantNameAttr(name string) attribute.KeyValue {
	return attribute.String(TenantNameKey, name)
}

func ResourceAttr(resource string) attribute.KeyValue {
	return attribute.String(ResourceKey, resource)
}

func CurrentAttr(current int64) attribute.KeyValue {
	return attribute.Int64(CurrentKey, current)
}

func LimitAttr(limit int64) attribute.KeyValue {
	return attribute.Int64(LimitKey, limit)
}

func OperationAttr(op string) attribute.KeyValue {
	return attribute.String(OperationKey, op)
}

func TargetTenantAttr(id string) attribute.KeyValue {
	return attribute.String(TargetTenantKey, id)
}

func TenantAttrs(tenantID string, extra ...attribute.KeyValue) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 1+len(extra))
	attrs = append(attrs, TenantIDAttr(tenantID))
	attrs = append(attrs, extra...)
	return attrs
}

func SetSpanTenantAttrs(span trace.Span, tenantID string, extra ...attribute.KeyValue) {
	if span == nil || tenantID == "" {
		return
	}
	attrs := TenantAttrs(tenantID, extra...)
	span.SetAttributes(attrs...)
}

type TenantMetrics struct {
	quotaUtilization metric.Int64Gauge
	quotaViolations  metric.Int64Counter
	meter            metric.Meter
}

// NewTenantMetrics creates a TenantMetrics instance with quota utilization and violation metrics. It returns nil if the provided meter is nil, and returns an error if metric creation fails.
func NewTenantMetrics(meter metric.Meter) (*TenantMetrics, error) {
	if meter == nil {
		return nil, nil
	}

	quotaUtilization, err := meter.Int64Gauge("ayb_tenant_quota_utilization",
		metric.WithDescription("Current quota utilization for tenant resources"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	quotaViolations, err := meter.Int64Counter("ayb_tenant_quota_violations_total",
		metric.WithDescription("Total count of quota enforcement rejections"),
	)
	if err != nil {
		return nil, err
	}

	return &TenantMetrics{
		quotaUtilization: quotaUtilization,
		quotaViolations:  quotaViolations,
		meter:            meter,
	}, nil
}

func (m *TenantMetrics) RecordQuotaUtilization(ctx context.Context, tenantID, resource string, current, limit int64) {
	if m == nil || m.quotaUtilization == nil || tenantID == "" || limit <= 0 {
		return
	}
	utilization := (current * 100) / limit
	m.quotaUtilization.Record(ctx, utilization, metric.WithAttributes(
		TenantIDAttr(tenantID),
		ResourceAttr(resource),
	))
}

func (m *TenantMetrics) IncrQuotaViolation(ctx context.Context, tenantID, resource string) {
	if m == nil || m.quotaViolations == nil || tenantID == "" {
		return
	}
	m.quotaViolations.Add(ctx, 1, metric.WithAttributes(
		TenantIDAttr(tenantID),
		ResourceAttr(resource),
	))
}
