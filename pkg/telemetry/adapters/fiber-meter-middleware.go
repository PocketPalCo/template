package adapters

import (
	"context"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
	otelcontrib "go.opentelemetry.io/contrib"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"net/http"
	"time"
)

const (
	tracerKey           = "service.tracer"
	instrumentationName = "github.com/gofiber/contrib/otelfiber"

	MetricNameHttpServerDuration       = "http.server.duration"
	MetricNameHttpServerRequestSize    = "http.server.request.size"
	MetricNameHttpServerResponseSize   = "http.server.response.size"
	MetricNameHttpServerActiveRequests = "http.server.active_requests"

	// Unit constants for deprecated metric units
	UnitDimensionless = "1"
	UnitBytes         = "By"
	UnitMilliseconds  = "ms"
)

var (
	httpProtocolNameAttr = semconv.NetworkProtocolName("http")
	http11VersionAttr    = semconv.NetworkProtocolVersion("1.1")
	http10VersionAttr    = semconv.NetworkProtocolVersion("1.0")
)

func WithInstrumentation(metricProvider *metric.MeterProvider, tracerProvider *sdktrace.TracerProvider) fiber.Handler {
	tracer := tracerProvider.Tracer(
		instrumentationName,
		oteltrace.WithInstrumentationVersion(otelcontrib.Version()),
	)

	meter := metricProvider.Meter("http")

	httpServerDuration, err := meter.Float64Histogram(MetricNameHttpServerDuration, api.WithUnit(UnitMilliseconds), api.WithDescription("measures the duration inbound HTTP requests"))
	if err != nil {
		otel.Handle(err)
	}
	httpServerRequestSize, err := meter.Int64Histogram(MetricNameHttpServerRequestSize, api.WithUnit(UnitBytes), api.WithDescription("measures the size of HTTP request messages"))
	if err != nil {
		otel.Handle(err)
	}
	httpServerResponseSize, err := meter.Int64Histogram(MetricNameHttpServerResponseSize, api.WithUnit(UnitBytes), api.WithDescription("measures the size of HTTP response messages"))
	if err != nil {
		otel.Handle(err)
	}
	httpServerActiveRequests, err := meter.Int64UpDownCounter(MetricNameHttpServerActiveRequests, api.WithUnit(UnitDimensionless), api.WithDescription("measures the number of concurrent HTTP requests that are currently in-flight"))
	if err != nil {
		otel.Handle(err)
	}

	return func(c *fiber.Ctx) error {
		c.Locals(tracerKey, tracer)
		savedCtx, cancel := context.WithCancel(c.UserContext())
		start := time.Now()
		requestMetricsAttrs := httpServerMetricAttributesFromRequest(c)
		httpServerActiveRequests.Add(savedCtx, 1, api.WithAttributes(requestMetricsAttrs...))

		responseMetricAttrs := make([]attribute.KeyValue, len(requestMetricsAttrs))
		copy(responseMetricAttrs, requestMetricsAttrs)

		reqHeader := make(http.Header)

		c.Request().Header.VisitAll(func(k, v []byte) {
			reqHeader.Add(string(k), string(v))
		})

		opts := []oteltrace.SpanStartOption{
			oteltrace.WithAttributes(httpServerTraceAttributesFromRequest(c)...),
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		}

		spanName := utils.CopyString(c.Path())
		ctx, span := tracer.Start(savedCtx, spanName, opts...)
		defer span.End()

		c.SetUserContext(ctx)

		if err := c.Next(); err != nil {
			span.RecordError(err)
			// invokes the registered HTTP error handler
			// to get the correct response status code
			_ = c.App().Config().ErrorHandler(c, err)
		}

		responseAttrs := []attribute.KeyValue{
			semconv.HTTPResponseStatusCode(c.Response().StatusCode()),
			semconv.HTTPRouteKey.String(c.Route().Path), // no need to copy c.Route().Path: route strings should be immutable across app lifecycle
		}

		var responseSize int64
		requestSize := int64(len(c.Request().Body()))
		if c.GetRespHeader("Content-Type") != "text/event-stream" {
			responseSize = int64(len(c.Response().Body()))
		}

		defer func() {
			responseMetricAttrs = append(responseMetricAttrs, responseAttrs...)

			httpServerActiveRequests.Add(savedCtx, -1, api.WithAttributes(requestMetricsAttrs...))
			httpServerDuration.Record(savedCtx, float64(time.Since(start).Microseconds())/1000, api.WithAttributes(responseMetricAttrs...))
			httpServerRequestSize.Record(savedCtx, requestSize, api.WithAttributes(responseMetricAttrs...))
			httpServerResponseSize.Record(savedCtx, responseSize, api.WithAttributes(responseMetricAttrs...))

			c.SetUserContext(savedCtx)
			cancel()
		}()

		span.SetAttributes(append(responseAttrs, semconv.HTTPResponseBodySizeKey.Int64(responseSize))...)

		spanStatus, spanMessage := spanStatusFromHTTPStatusCodeAndSpanKind(c.Response().StatusCode(), oteltrace.SpanKindServer)
		span.SetStatus(spanStatus, spanMessage)

		//Propagate tracing context as headers in outbound response
		tracingHeaders := make(propagation.HeaderCarrier)
		for _, headerKey := range tracingHeaders.Keys() {
			c.Set(headerKey, tracingHeaders.Get(headerKey))
		}

		return nil
	}
}

func httpServerTraceAttributesFromRequest(c *fiber.Ctx) []attribute.KeyValue {
	protocolAttributes := httpNetworkProtocolAttributes(c)
	attrs := []attribute.KeyValue{
		// utils.CopyString: we need to copy the string as fasthttp strings are by default
		// mutable so it will be unsafe to use in this middleware as it might be used after
		// the handler returns.
		semconv.HTTPRequestMethodKey.String(utils.CopyString(c.Method())),
		semconv.URLScheme(utils.CopyString(c.Protocol())),
		semconv.HTTPRequestBodySize(c.Request().Header.ContentLength()),
		semconv.URLPath(string(utils.CopyBytes(c.Request().URI().Path()))),
		semconv.URLQuery(c.Request().URI().QueryArgs().String()),
		semconv.URLFull(utils.CopyString(c.OriginalURL())),
		semconv.UserAgentOriginal(string(utils.CopyBytes(c.Request().Header.UserAgent()))),
		semconv.ServerAddress(utils.CopyString(c.Hostname())),
	}
	attrs = append(attrs, protocolAttributes...)

	clientIP := c.IP()
	if len(clientIP) > 0 {
		attrs = append(attrs, semconv.ClientAddress(utils.CopyString(clientIP)))
	}

	return attrs
}

func httpNetworkProtocolAttributes(c *fiber.Ctx) []attribute.KeyValue {
	httpProtocolAttributes := []attribute.KeyValue{httpProtocolNameAttr}
	if c.Request().Header.IsHTTP11() {
		return append(httpProtocolAttributes, http11VersionAttr)
	}
	return append(httpProtocolAttributes, http10VersionAttr)
}

func httpServerMetricAttributesFromRequest(c *fiber.Ctx) []attribute.KeyValue {
	protocolAttributes := httpNetworkProtocolAttributes(c)
	attrs := []attribute.KeyValue{
		semconv.URLScheme(utils.CopyString(c.Protocol())),
		semconv.ServerAddress(utils.CopyString(c.Hostname())),
		semconv.HTTPRequestMethodKey.String(utils.CopyString(c.Method())),
	}
	attrs = append(attrs, protocolAttributes...)

	return attrs
}

func spanStatusFromHTTPStatusCodeAndSpanKind(code int, spanKind oteltrace.SpanKind) (codes.Code, string) {
	// This code block ignores the HTTP 306 status code. The 306 status code is no longer in use.
	if http.StatusText(code) == "" {
		return codes.Error, fmt.Sprintf("Invalid HTTP status code %d", code)
	}

	if (code >= http.StatusContinue && code < http.StatusBadRequest) ||
		(spanKind == oteltrace.SpanKindServer && isCode4xx(code)) {
		return codes.Unset, ""
	}
	return codes.Error, ""
}

func isCode4xx(code int) bool {
	return code >= http.StatusBadRequest && code <= http.StatusUnavailableForLegalReasons
}
