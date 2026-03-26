package observability

import "go.opentelemetry.io/otel/attribute"

func methodAttr(method string) attribute.KeyValue {
	return attribute.String("method", method)
}

func routeAttr(route string) attribute.KeyValue {
	return attribute.String("route", route)
}

func statusAttr(status string) attribute.KeyValue {
	return attribute.String("status", status)
}
