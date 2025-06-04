package server

import (
	"context"
	"github.com/PocketPalCo/shopping-service/config"
	"github.com/PocketPalCo/shopping-service/internal/infra/postgres"
	"github.com/PocketPalCo/shopping-service/pkg/telemetry"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	"google.golang.org/grpc"
	"log/slog"
	"time"
)

type Server struct {
	cfg            *config.Config
	app            *fiber.App
	db             postgres.DB
	traceProvider  *sdktrace.TracerProvider
	metricProvider *metric.MeterProvider
}

func New(ctx context.Context, cfg *config.Config, dbConn *pgxpool.Pool) *Server {
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OtlpEndpoint),
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithDialOption(grpc.WithUserAgent("shopping-service")),
	)
	if err != nil {
		slog.Error("failed to initialize otlp trace exporter", slog.String("error", err.Error()))
		return nil
	}
	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OtlpEndpoint),
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithDialOption(grpc.WithUserAgent("shopping-service")),
	)
	if err != nil {
		slog.Error("failed to initialize otlp exporter", slog.String("error", err.Error()))
		return nil
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String("service-name"),
			)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	provider := metric.NewMeterProvider(metric.WithResource(resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String("shopping-service"),
	)), metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(15*time.Second))))

	otel.SetMeterProvider(provider)

	err = telemetry.InitTelemetry(provider, dbConn)
	if err != nil {
		slog.Error("failed to initialize telemetry", slog.String("error", err.Error()))
		return nil
	}

	instrumentedConn, err := telemetry.NewInstrumentedPool(provider, dbConn)
	if err != nil {
		slog.Error("failed to create instrumented pool", slog.String("error", err.Error()))
	}

	app := fiber.New()

	return &Server{
		cfg:            cfg,
		app:            app,
		db:             instrumentedConn,
		traceProvider:  tp,
		metricProvider: provider,
	}
}

func (s *Server) Shutdown() {
	slog.Info("Shutting down server")

	if err := s.traceProvider.Shutdown(context.Background()); err != nil {
		slog.Error("Error shutting down trace provider", slog.String("error", err.Error()))
	}

	if err := s.metricProvider.Shutdown(context.Background()); err != nil {
		slog.Error("Error shutting down metric provider", slog.String("error", err.Error()))
	}

	s.db.Close()

	if err := s.app.Shutdown(); err != nil {
		slog.Error("Error shutting down server", slog.String("error", err.Error()))
	}

	slog.Info("Http Server shut down successfully")
}

func (s *Server) Start() {
	initGlobalMiddlewares(s.app, s.cfg)
	registerHttpRoutes(s.app, s.cfg, s.db)

	setupWs(s.app, s.cfg, s.db)

	slog.Info("Starting server", slog.String("address", s.cfg.ServerAddress))

	err := s.app.Listen(":8080")
	if err != nil {
		return
	}
}
