package tracer

import (
	"context"

	"gitlab.ozon.dev/gojhw1/pkg/config"
	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func InitTracerProvider(ctx context.Context, cfg *config.Config) (func(), error) {
	logger.Infof("Инициализация OTLP трейсера: endpoint=%s, service=%s", cfg.Jaeger.OtlpEndpoint, cfg.Jaeger.ServiceName)

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.Jaeger.OtlpEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		logger.Errorf("Ошибка создания OTLP трейсера: %v", err)
		return nil, err
	}

	tracerProvider := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exporter),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(cfg.Jaeger.ServiceName),
			semconv.ServiceVersionKey.String("1.0.0"),
		)),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	logger.Info("OTLP трейсер успешно инициализирован")

	return func() {
		if err := tracerProvider.Shutdown(ctx); err != nil {
			logger.Errorf("Ошибка завершения OTLP трейсера: %v", err)
		} else {
			logger.Info("OTLP трейсер успешно завершен")
		}
	}, nil
}
