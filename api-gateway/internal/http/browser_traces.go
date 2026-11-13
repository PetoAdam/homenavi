package http

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/observability"
	oteltrace "go.opentelemetry.io/otel/trace"
	collectortracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"golang.org/x/net/http/httpguts"
	"google.golang.org/protobuf/proto"
)

const browserTraceMaxBytes = 256 << 10

func browserTraceHandler(endpoint string) http.Handler {
	client, clientErr := browserCollectorClient()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if clientErr != nil {
			http.Error(w, "trace collector TLS is not configured", http.StatusServiceUnavailable)
			return
		}
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
			http.Error(w, "trace collector is not configured", http.StatusServiceUnavailable)
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || (mediaType != "application/json" && mediaType != "application/x-protobuf") {
			http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, browserTraceMaxBytes))
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				http.Error(w, "trace payload too large", http.StatusRequestEntityTooLarge)
			} else {
				http.Error(w, "invalid trace payload", http.StatusBadRequest)
			}
			return
		}
		body, err = normalizeBrowserTraces(body, mediaType)
		if err != nil {
			http.Error(w, "invalid trace payload", http.StatusBadRequest)
			return
		}
		outbound, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			http.Error(w, "invalid trace collector", http.StatusServiceUnavailable)
			return
		}
		if err := applyCollectorHeaders(outbound.Header); err != nil {
			http.Error(w, "invalid trace collector headers", http.StatusServiceUnavailable)
			return
		}
		outbound.Header.Set("Content-Type", mediaType)
		response, err := client.Do(outbound)
		if err != nil {
			http.Error(w, "trace collector unavailable", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		if contentType := response.Header.Get("Content-Type"); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, io.LimitReader(response.Body, 64<<10))
	})
}

func browserCollectorClient() (*http.Client, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	caFile := os.Getenv("OTEL_EXPORTER_OTLP_CERTIFICATE")
	if value := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE"); value != "" {
		caFile = value
	}
	if caFile != "" {
		data, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(data) {
			return nil, errors.New("invalid collector CA certificate")
		}
		tlsConfig.RootCAs = roots
	}
	certFile, keyFile := os.Getenv("OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE"), os.Getenv("OTEL_EXPORTER_OTLP_CLIENT_KEY")
	traceCert, traceKey := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE"), os.Getenv("OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY")
	if traceCert != "" || traceKey != "" {
		certFile, keyFile = traceCert, traceKey
	}
	if (certFile == "") != (keyFile == "") {
		return nil, errors.New("collector client certificate and key are required together")
	}
	if certFile != "" {
		certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = defaultTransport.Clone()
	}
	transport.TLSClientConfig = tlsConfig
	return &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func applyCollectorHeaders(headers http.Header) error {
	value := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")
	if traceHeaders := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS"); traceHeaders != "" {
		value = traceHeaders
	}
	if value == "" {
		return nil
	}
	for _, entry := range strings.Split(value, ",") {
		name, encoded, ok := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		decoded, err := url.PathUnescape(encoded)
		decoded = strings.TrimSpace(decoded)
		if !ok || err != nil || !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(decoded) {
			return errors.New("invalid collector header")
		}
		headers.Set(name, decoded)
	}
	return nil
}

func normalizeBrowserTraces(body []byte, mediaType string) ([]byte, error) {
	if mediaType == "application/x-protobuf" {
		var request collectortracev1.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			return nil, err
		}
		for _, resourceSpans := range request.ResourceSpans {
			resourceSpans.Resource = &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{
				Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "homenavi-frontend"}},
			}, {Key: "homenavi.telemetry.source", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "browser"}}}}}
			for _, scopeSpans := range resourceSpans.ScopeSpans {
				scopeSpans.Scope = &commonv1.InstrumentationScope{Name: "homenavi-browser"}
				for _, span := range scopeSpans.Spans {
					span.Kind = tracev1.Span_SPAN_KIND_CLIENT
					if observability.IndependentIngressEnabled() {
						if len(span.TraceId) != 16 {
							return nil, errors.New("invalid trace ID")
						}
						var id oteltrace.TraceID
						copy(id[:], span.TraceId)
						if !id.IsValid() {
							return nil, errors.New("invalid trace ID")
						}
						namespaced := observability.NamespaceIngressTraceID(id)
						span.TraceId = namespaced[:]
						span.Links = nil
					}
				}
			}
		}
		return proto.Marshal(&request)
	}

	type scopeSpans struct {
		Scope          json.RawMessage              `json:"scope"`
		Spans          []map[string]json.RawMessage `json:"spans"`
		SchemaURL      json.RawMessage              `json:"schemaUrl,omitempty"`
		ProtoSchemaURL json.RawMessage              `json:"schema_url,omitempty"`
	}
	type resourceSpans struct {
		Resource        json.RawMessage `json:"resource"`
		ScopeSpans      []scopeSpans    `json:"scopeSpans"`
		ProtoScopeSpans []scopeSpans    `json:"scope_spans,omitempty"`
		SchemaURL       json.RawMessage `json:"schemaUrl,omitempty"`
		ProtoSchemaURL  json.RawMessage `json:"schema_url,omitempty"`
	}
	var request struct {
		ResourceSpans      []resourceSpans `json:"resourceSpans"`
		ProtoResourceSpans []resourceSpans `json:"resource_spans,omitempty"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, err
	}
	if request.ProtoResourceSpans != nil {
		if request.ResourceSpans != nil {
			return nil, errors.New("ambiguous resource spans")
		}
		request.ResourceSpans, request.ProtoResourceSpans = request.ProtoResourceSpans, nil
	}
	for index := range request.ResourceSpans {
		resourceSpans := &request.ResourceSpans[index]
		if resourceSpans.ProtoSchemaURL != nil {
			if resourceSpans.SchemaURL != nil {
				return nil, errors.New("ambiguous resource schema URL")
			}
			resourceSpans.SchemaURL, resourceSpans.ProtoSchemaURL = resourceSpans.ProtoSchemaURL, nil
		}
		if resourceSpans.ProtoScopeSpans != nil {
			if resourceSpans.ScopeSpans != nil {
				return nil, errors.New("ambiguous scope spans")
			}
			resourceSpans.ScopeSpans, resourceSpans.ProtoScopeSpans = resourceSpans.ProtoScopeSpans, nil
		}
		resourceSpans.Resource = json.RawMessage(`{"attributes":[{"key":"service.name","value":{"stringValue":"homenavi-frontend"}},{"key":"homenavi.telemetry.source","value":{"stringValue":"browser"}}]}`)
		for scopeIndex := range resourceSpans.ScopeSpans {
			scopeSpans := &resourceSpans.ScopeSpans[scopeIndex]
			if scopeSpans.ProtoSchemaURL != nil {
				if scopeSpans.SchemaURL != nil {
					return nil, errors.New("ambiguous scope schema URL")
				}
				scopeSpans.SchemaURL, scopeSpans.ProtoSchemaURL = scopeSpans.ProtoSchemaURL, nil
			}
			scopeSpans.Scope = json.RawMessage(`{"name":"homenavi-browser"}`)
			for _, span := range scopeSpans.Spans {
				if span == nil {
					return nil, errors.New("invalid span")
				}
				span["kind"] = json.RawMessage(`3`)
				if observability.IndependentIngressEnabled() {
					if len(span["traceId"]) > 0 && len(span["trace_id"]) > 0 {
						return nil, errors.New("ambiguous trace ID")
					}
					traceID := span["traceId"]
					if len(traceID) == 0 {
						traceID = span["trace_id"]
					}
					var encoded string
					if err := json.Unmarshal(traceID, &encoded); err != nil {
						return nil, err
					}
					id, err := oteltrace.TraceIDFromHex(encoded)
					if err != nil {
						return nil, err
					}
					span["traceId"], _ = json.Marshal(observability.NamespaceIngressTraceID(id).String())
					delete(span, "trace_id")
					delete(span, "links")
				}
			}
		}
	}
	return json.Marshal(request)
}

func browserTraceEnabled(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}
