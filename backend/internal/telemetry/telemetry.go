// Package telemetry provides OpenTelemetry tracing, logging, and metrics setup
// and utilities.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	otellog "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	oteltrace "go.opentelemetry.io/otel/trace"
)

var tracer oteltrace.Tracer

// Config holds telemetry configuration.
type Config struct {
	Enabled  bool
	Endpoint string
	Insecure bool
}

// InitTracing initializes OpenTelemetry tracing.
func InitTracing(ctx context.Context, serviceName, serviceVersion string, cfg Config, log *slog.Logger) (func(), error) {
	if !cfg.Enabled {
		log.InfoContext(ctx, "Telemetry is disabled")
		return func() {}, nil
	}

	var exporter trace.SpanExporter
	if cfg.Endpoint == "" || cfg.Endpoint == "stdout" {
		// Use stdout for development/debugging
		var err error
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout exporter: %w", err)
		}
		log.InfoContext(ctx, "Using stdout trace exporter")
	} else {
		// Use OTLP for production-like setup
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.Endpoint),
		}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithGRPCConn(
				mustCreateGRPCConn(cfg.Endpoint),
			))
		}

		var err error
		exporter, err = otlptracegrpc.New(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
		log.InfoContext(ctx, "Using OTLP trace exporter", "endpoint", cfg.Endpoint)
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(newResource(ctx, serviceName, serviceVersion)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = tp.Tracer("github.com/zorcal/theapp/backend")

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.ErrorContext(shutdownCtx, "shutting down OpenTelemetry tracer provider", "error", err)
		}
	}

	return cleanup, nil
}

// InitLogging initializes OpenTelemetry logging and returns an slog.Handler
// that forwards records to the configured OTLP endpoint. Records below
// minLevel are dropped before export so we don't ship DEBUG chatter to the
// remote backend in production. When telemetry is disabled the handler is
// nil — callers should treat that as "skip the OTel branch of any
// multi-handler" rather than as an error.
func InitLogging(ctx context.Context, serviceName, serviceVersion string, cfg Config, minLevel slog.Leveler, log *slog.Logger) (slog.Handler, func(), error) {
	if !cfg.Enabled {
		return nil, func() {}, nil
	}

	opts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	}

	exporter, err := otlploggrpc.New(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create OTLP log exporter: %w", err)
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(newResource(ctx, serviceName, serviceVersion)),
	)
	otellog.SetLoggerProvider(lp)

	log.InfoContext(ctx, "Using OTLP log exporter", "endpoint", cfg.Endpoint, "min_level", minLevel.Level())

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := lp.Shutdown(shutdownCtx); err != nil {
			log.ErrorContext(shutdownCtx, "shutting down OpenTelemetry logger provider", "error", err)
		}
	}

	h := &slogLevelHandler{leveler: minLevel, next: otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(lp))}
	return h, cleanup, nil
}

// InitMetrics initializes OpenTelemetry metrics, exporting via OTLP to the
// configured endpoint on a periodic interval, and starts Go runtime
// instrumentation (GC pauses, goroutine count, memory). When telemetry is
// disabled it is a no-op. gRPC server metrics need no extra wiring: the
// otelgrpc stats handler already installed on the server reads whatever
// MeterProvider is registered globally, so setting it here is what turns those
// metrics on.
func InitMetrics(ctx context.Context, serviceName, serviceVersion string, cfg Config, log *slog.Logger) (func(), error) {
	if !cfg.Enabled {
		return func() {}, nil
	}

	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(newResource(ctx, serviceName, serviceVersion)),
	)
	otel.SetMeterProvider(mp)

	if err := runtime.Start(runtime.WithMeterProvider(mp)); err != nil {
		return nil, fmt.Errorf("start runtime metrics: %w", err)
	}

	log.InfoContext(ctx, "Using OTLP metric exporter", "endpoint", cfg.Endpoint)

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := mp.Shutdown(shutdownCtx); err != nil {
			log.ErrorContext(shutdownCtx, "shutting down OpenTelemetry meter provider", "error", err)
		}
	}

	return cleanup, nil
}

// GetTraceID retrieves the trace ID from the current span in the context.
func GetTraceID(ctx context.Context) string {
	spanctx := oteltrace.SpanFromContext(ctx).SpanContext()
	if !spanctx.IsValid() {
		return ""
	}
	return spanctx.TraceID().String()
}

// GetSpanID retrieves the span ID from the current span in the context.
func GetSpanID(ctx context.Context) string {
	spanctx := oteltrace.SpanFromContext(ctx).SpanContext()
	if !spanctx.IsValid() {
		return ""
	}
	return spanctx.SpanID().String()
}

// StartSpan starts a new span with the given name and returns the context and span.
// If the tracer hasn't been initialized (e.g., in tests), returns a no-op span.
func StartSpan(ctx context.Context, name string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	if tracer == nil {
		return ctx, oteltrace.SpanFromContext(ctx)
	}
	return tracer.Start(ctx, name, opts...)
}

// SpanFromContext returns the current span from the context.
func SpanFromContext(ctx context.Context) oteltrace.Span {
	return oteltrace.SpanFromContext(ctx)
}

// SetBaggage sets a key-value pair in baggage that will be automatically
// propagated to all child spans and across service boundaries.
func SetBaggage(ctx context.Context, key, value string) context.Context {
	member, err := baggage.NewMember(key, value)
	if err != nil {
		return ctx
	}

	bag, err := baggage.FromContext(ctx).SetMember(member)
	if err != nil {
		return ctx
	}

	return baggage.ContextWithBaggage(ctx, bag)
}

// GetBaggage retrieves a value from baggage.
func GetBaggage(ctx context.Context, key string) string {
	return baggage.FromContext(ctx).Member(key).Value()
}

// StartSpanWithBaggageAttrs starts a new span and automatically adds all
// baggage items as span attributes. If the tracer hasn't been initialized
// (e.g., in tests), returns a no-op span.
func StartSpanWithBaggageAttrs(ctx context.Context, name string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	if tracer == nil {
		return ctx, oteltrace.SpanFromContext(ctx)
	}

	ctx, span := tracer.Start(ctx, name, opts...)

	bag := baggage.FromContext(ctx)
	for _, member := range bag.Members() {
		span.SetAttributes(attribute.String(member.Key(), member.Value()))
	}

	return ctx, span
}

// mustCreateGRPCConn creates an insecure gRPC connection for local development.
func mustCreateGRPCConn(endpoint string) *grpc.ClientConn {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(fmt.Sprintf("failed to create gRPC connection: %v", err))
	}
	return conn
}

func newResource(ctx context.Context, serviceName, serviceVersion string) *resource.Resource {
	r, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
		resource.WithContainer(),
		resource.WithHost(),
	)
	if err != nil {
		return resource.Default()
	}
	return r
}

// slogLevelHandler wraps an slog.Handler with a minimum-level threshold.
// otelslog has no level option of its own, and slog-multi asks each branch's
// Enabled independently — so without this, the OTel branch would ship every
// record regardless of what level the stdout branch is set to.
type slogLevelHandler struct {
	leveler slog.Leveler
	next    slog.Handler
}

func (h *slogLevelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if level < h.leveler.Level() {
		return false
	}
	return h.next.Enabled(ctx, level)
}

func (h *slogLevelHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.next.Handle(ctx, r)
}

func (h *slogLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &slogLevelHandler{leveler: h.leveler, next: h.next.WithAttrs(attrs)}
}

func (h *slogLevelHandler) WithGroup(name string) slog.Handler {
	return &slogLevelHandler{leveler: h.leveler, next: h.next.WithGroup(name)}
}
