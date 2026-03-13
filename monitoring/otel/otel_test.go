package otel

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracetest "go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestFlyAttributesIncludesConfiguredValues(t *testing.T) {
	t.Setenv(flyMachineIDEnv, "machine-123")
	t.Setenv(flyAppNameEnv, "851-imgproxy")
	t.Setenv(flyRegionEnv, "iad")

	hostnameFunc = func() (string, error) { return "machine-host", nil }
	t.Cleanup(func() { hostnameFunc = os.Hostname })

	attrs := flyAttributes()

	assertAttributeMap(t, attrs, map[string]string{
		"fly.machine.id":       "machine-123",
		"fly.app.name":         "851-imgproxy",
		"fly.region":           "iad",
		"fly.machine.hostname": "machine-host",
	})
}

func TestFlyAttributesSkipsEmptyValues(t *testing.T) {
	hostnameFunc = func() (string, error) { return "", fmt.Errorf("no hostname") }
	t.Cleanup(func() { hostnameFunc = os.Hostname })

	attrs := flyAttributes()
	if len(attrs) != 0 {
		t.Fatalf("expected no fly attributes, got %d", len(attrs))
	}
}

func TestStartRequestAddsFlyAttributesToRootSpan(t *testing.T) {
	t.Setenv(flyMachineIDEnv, "machine-123")
	t.Setenv(flyAppNameEnv, "851-imgproxy")
	t.Setenv(flyRegionEnv, "iad")

	hostnameFunc = func() (string, error) { return "machine-host", nil }
	t.Cleanup(func() { hostnameFunc = os.Hostname })

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	o := &Otel{
		tracerProvider: provider,
		tracer:         provider.Tracer("imgproxy-test"),
	}

	request := httptest.NewRequest(http.MethodGet, "https://imgproxy.attic.sh/insecure/plain/example.png", nil)
	recorder := httptest.NewRecorder()

	ctx, cancel, responseWriter := o.StartRequest(context.Background(), recorder, request)
	responseWriter.WriteHeader(http.StatusOK)
	cancel()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}

	if got := spans[0].Name; got != "/request" {
		t.Fatalf("expected root span name /request, got %q", got)
	}

	attrs := spanAttributes(spans[0].Attributes)
	if attrs["fly.machine.id"] != "machine-123" {
		t.Fatalf("expected fly.machine.id on root span, got %q", attrs["fly.machine.id"])
	}
	if attrs["fly.app.name"] != "851-imgproxy" {
		t.Fatalf("expected fly.app.name on root span, got %q", attrs["fly.app.name"])
	}
	if attrs["fly.region"] != "iad" {
		t.Fatalf("expected fly.region on root span, got %q", attrs["fly.region"])
	}
	if attrs["fly.machine.hostname"] != "machine-host" {
		t.Fatalf("expected fly.machine.hostname on root span, got %q", attrs["fly.machine.hostname"])
	}
	if !trace.SpanFromContext(ctx).SpanContext().IsValid() {
		t.Fatalf("expected valid span context on request context")
	}
}

func assertAttributeMap(t *testing.T, attrs []attribute.KeyValue, expected map[string]string) {
	t.Helper()

	actual := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		actual[string(attr.Key)] = attr.Value.AsString()
	}

	if len(actual) != len(expected) {
		t.Fatalf("expected %d attributes, got %d", len(expected), len(actual))
	}

	for key, value := range expected {
		if actual[key] != value {
			t.Fatalf("expected %s=%q, got %q", key, value, actual[key])
		}
	}
}

func spanAttributes(attrs []attribute.KeyValue) map[string]string {
	values := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		values[string(attr.Key)] = attr.Value.AsString()
	}
	return values
}
