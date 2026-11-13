from __future__ import annotations

import logging
import os
import threading

from fastapi import FastAPI, Request
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.trace import SpanKind, Status, StatusCode
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator

TRACING_SHUTDOWN_TIMEOUT = 2.0


def configure_tracing(app: FastAPI) -> None:
    try:
        provider = TracerProvider(resource=Resource.create({"service.name": "profile-picture-service"}), shutdown_on_exit=False)
    except Exception:
        logging.getLogger(__name__).warning("OpenTelemetry tracing unavailable")
        return
    endpoint = os.getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "").strip()
    if endpoint:
        try:
            provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=endpoint)))
        except Exception:
            logging.getLogger(__name__).warning("OpenTelemetry trace export unavailable")
    app.state.tracer_provider = provider
    tracer = provider.get_tracer("homenavi/profile-picture")
    propagator = TraceContextTextMapPropagator()

    @app.middleware("http")
    async def trace_request(request: Request, call_next):
        context = propagator.extract(request.headers)
        with tracer.start_as_current_span(
            "HTTP " + request.method,
            context=context,
            kind=SpanKind.SERVER,
            record_exception=False,
            set_status_on_exception=False,
        ) as span:
            span.set_attribute("http.method", request.method)
            try:
                response = await call_next(request)
            except Exception:
                span.set_status(Status(StatusCode.ERROR, "HTTP request failed"))
                raise
            span.set_attribute("http.status_code", response.status_code)
            if response.status_code >= 500:
                span.set_status(Status(StatusCode.ERROR, "HTTP server error"))
            response.headers["Trace-ID"] = format(span.get_span_context().trace_id, "032x")
            return response

    def shutdown_tracing() -> None:
        worker = threading.Thread(target=provider.shutdown, daemon=True)
        worker.start()
        worker.join(timeout=TRACING_SHUTDOWN_TIMEOUT)

    app.add_event_handler("shutdown", shutdown_tracing)