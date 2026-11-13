import { context, propagation, trace } from '@opentelemetry/api';
import { W3CTraceContextPropagator } from '@opentelemetry/core';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import { registerInstrumentations } from '@opentelemetry/instrumentation';
import { FetchInstrumentation } from '@opentelemetry/instrumentation-fetch';
import { XMLHttpRequestInstrumentation } from '@opentelemetry/instrumentation-xml-http-request';
import { resourceFromAttributes } from '@opentelemetry/resources';
import { BatchSpanProcessor, ParentBasedSampler, StackContextManager, TraceIdRatioBasedSampler, TracerProvider } from '@opentelemetry/sdk-trace';

const collectorPath = '/api/telemetry/traces';

export function ignoredBrowserTraceUrls(origin) {
  const escapedOrigin = origin.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return [new RegExp(`^(?!${escapedOrigin}/api/(?!telemetry/traces(?:[?#]|$)))`)];
}

export function initializeBrowserTracing({ enabled, sampleRate = 0.1, origin } = {}) {
  if (!enabled) return null;
  origin ||= window.location.origin;
  const ratio = Number(sampleRate);
  const safeRatio = Number.isFinite(ratio) && ratio >= 0 && ratio <= 1 ? ratio : 0.1;
  const provider = new TracerProvider({
    resource: resourceFromAttributes({ 'service.name': 'homenavi-frontend' }),
    sampler: new ParentBasedSampler({ root: new TraceIdRatioBasedSampler(safeRatio) }),
    spanLimits: { attributeCountLimit: 16, attributeValueLengthLimit: 512, eventCountLimit: 8 },
    spanProcessors: [new BatchSpanProcessor({
      exporter: new OTLPTraceExporter({ url: `${origin}${collectorPath}` }),
      maxExportBatchSize: 64,
      maxQueueSize: 512,
    })],
  });
  trace.setGlobalTracerProvider(provider);
  context.setGlobalContextManager(new StackContextManager().enable());
  propagation.setGlobalPropagator(new W3CTraceContextPropagator());

  const redactURL = span => span.setAttribute('url.full', `${origin}/api`);
  const options = {
    ignoreUrls: ignoredBrowserTraceUrls(origin),
    ignoreNetworkEvents: true,
    requestHook: redactURL,
    applyCustomAttributesOnSpan: redactURL,
  };
  registerInstrumentations({
    tracerProvider: provider,
    instrumentations: [new FetchInstrumentation(options), new XMLHttpRequestInstrumentation(options)],
  });
  return provider;
}