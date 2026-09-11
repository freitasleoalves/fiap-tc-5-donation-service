package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// httpRequestsCounter e httpDurationHistogram são as métricas customizadas
// expostas ao Prometheus (via OTel Collector -> prometheusremotewrite),
// consumidas pelo dashboard SRE (SLO de taxa de erro e de latência p95).
var (
	httpRequestsCounter   metric.Int64Counter
	httpDurationHistogram metric.Float64Histogram
)

const serviceName = "donation-service"

// initOTel configura os providers de Trace e Métricas do OpenTelemetry,
// exportando via OTLP/gRPC para o OTel Collector (endpoint definido pelas
// variáveis padrão OTEL_EXPORTER_OTLP_ENDPOINT / OTEL_SERVICE_NAME, já
// setadas no Deployment do Kubernetes). Retorna uma função de shutdown.
func initOTel(ctx context.Context) (func(context.Context) error, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, err
	}

	// --- Traces ---
	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// --- Métricas ---
	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(15*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	meter := otel.Meter(serviceName)
	httpRequestsCounter, err = meter.Int64Counter(
		"solidarytech_http_requests_total",
		metric.WithDescription("Total de requisições HTTP recebidas, por método/rota/status"),
	)
	if err != nil {
		return nil, err
	}

	httpDurationHistogram, err = meter.Float64Histogram(
		"solidarytech_http_request_duration_seconds",
		metric.WithDescription("Duração das requisições HTTP, em segundos — base do SLI de latência (p95)"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return nil, err
	}

	log.Printf("OpenTelemetry inicializado para o serviço '%s'", serviceName)

	return func(shutdownCtx context.Context) error {
		if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return meterProvider.Shutdown(shutdownCtx)
	}, nil
}

// statusRecorder captura o status code da resposta para alimentar as
// métricas customizadas de requisições HTTP.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// withMetrics incrementa o contador solidarytech_http_requests_total e
// registra a duração no histograma solidarytech_http_request_duration_seconds
// a cada requisição, com os atributos service_name, http_method, http_route
// e http_status_code (usados no dashboard SRE).
func withMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start).Seconds()

		attrs := metric.WithAttributes(
			attribute.String("service_name", serviceName),
			attribute.String("http_method", r.Method),
			attribute.String("http_route", r.URL.Path),
			attribute.String("http_status_code", strconv.Itoa(rec.status)),
		)

		if httpRequestsCounter != nil {
			httpRequestsCounter.Add(r.Context(), 1, attrs)
		}
		if httpDurationHistogram != nil {
			httpDurationHistogram.Record(r.Context(), elapsed, attrs)
		}
	})
}

// instrumentHandler encapsula o handler HTTP raiz com o middleware de
// tracing automático do OTel (otelhttp) + as métricas customizadas acima.
func instrumentHandler(next http.Handler) http.Handler {
	return otelhttp.NewHandler(withMetrics(next), serviceName)
}

// mapCarrier adapta um map[string]string para o propagation.TextMapCarrier,
// usado para injetar o contexto de trace em mensagens de fila (Service
// Bus/SQS), já que elas não têm "headers HTTP".
type mapCarrier map[string]string

func (c mapCarrier) Get(key string) string { return c[key] }
func (c mapCarrier) Set(key, value string) { c[key] = value }
func (c mapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// injectTraceContext serializa o SpanContext ativo em ctx como um conjunto
// de propriedades de mensagem (padrão W3C traceparent/tracestate), para que
// o consumidor (notification-service) reconecte o span ao mesmo trace
// distribuído no APM.
func injectTraceContext(ctx context.Context) map[string]string {
	carrier := mapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier
}
