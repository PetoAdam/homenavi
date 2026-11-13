package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PetoAdam/homenavi/api-gateway/internal/gateway"
	"github.com/PetoAdam/homenavi/shared/observability"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func MakeRestProxyHandler(route gateway.RouteConfig) http.HandlerFunc {
	upstreamURL, err := url.Parse(route.Upstream)
	if err != nil {
		panic("Invalid upstream URL: " + route.Upstream)
	}
	routePrefix := strings.TrimSuffix(route.Path, "/*")
	upstreamPrefix := strings.TrimSuffix(upstreamURL.Path, "/*")
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("proxy request", "method", r.Method, "path", r.URL.Path, "upstream", upstreamURL.String())
		proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
		proxy.Transport = observability.InstrumentHTTPTransport(nil)
		proxy.ModifyResponse = func(response *http.Response) error {
			if w.Header().Get("Trace-ID") != "" {
				response.Header.Del("Trace-ID")
			}
			return nil
		}
		origDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			origDirector(req)
			ctx := chi.RouteContext(r.Context())
			path := upstreamURL.Path
			rawPath := ""
			// If the route uses a wildcard (/*), preserve the suffix after the route prefix.
			// This allows proxying IDs that contain slashes (e.g. zigbee/0x...) without URL-encoding.
			if strings.HasSuffix(route.Path, "/*") && strings.HasSuffix(upstreamURL.Path, "/*") {
				suffix := strings.TrimPrefix(r.URL.Path, routePrefix)
				if suffix == "" {
					suffix = "/"
				}
				path = upstreamPrefix + suffix
			} else if ctx != nil {
				rawPath = upstreamURL.EscapedPath()
				for i, key := range ctx.URLParams.Keys {
					val := ctx.URLParams.Values[i]
					if r.URL.RawPath != "" {
						if decoded, err := url.PathUnescape(val); err == nil {
							val = decoded
						}
					}
					path = strings.ReplaceAll(path, "{"+key+"}", val)
					rawPath = strings.ReplaceAll(rawPath, url.PathEscape("{"+key+"}"), url.PathEscape(val))
				}
			}
			req.URL.Path = path
			req.URL.RawPath = rawPath
			req.URL.RawQuery = r.URL.RawQuery
		}
		proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			slog.Error("proxy error", "method", r.Method, "path", r.URL.Path, "upstream", upstreamURL.String(), "error", err)
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(rw).Encode(map[string]any{"error": "upstream error", "code": http.StatusBadGateway})
		}
		proxy.ServeHTTP(w, r)
	}
}

func MakeWebSocketProxyHandler(route gateway.RouteConfig) http.HandlerFunc {
	upstreamURL, err := url.Parse(route.Upstream)
	if err != nil {
		panic("Invalid upstream URL: " + route.Upstream)
	}
	routePrefix := strings.TrimSuffix(route.Path, "/*")
	upstreamPrefix := strings.TrimSuffix(upstreamURL.Path, "/*")
	return func(w http.ResponseWriter, r *http.Request) {
		// Compute upstream URL per-request (support {params} and wildcard rewriting).
		u := *upstreamURL
		path := upstreamURL.Path
		ctx := chi.RouteContext(r.Context())
		if strings.HasSuffix(route.Path, "/*") && strings.HasSuffix(upstreamURL.Path, "/*") {
			suffix := strings.TrimPrefix(r.URL.Path, routePrefix)
			if suffix == "" {
				suffix = "/"
			}
			path = upstreamPrefix + suffix
		} else if ctx != nil {
			for i, key := range ctx.URLParams.Keys {
				val := ctx.URLParams.Values[i]
				path = strings.ReplaceAll(path, "{"+key+"}", val)
			}
		}
		u.Path = path
		u.RawQuery = r.URL.RawQuery

		slog.Info("proxy websocket", "path", r.URL.Path, "upstream", u.String())

		// Prepare headers for backend dial (forward subprotocols if any)
		dialHeader := http.Header{}
		if sp := r.Header.Get("Sec-WebSocket-Protocol"); sp != "" {
			// Forward exactly what client asked for; backend will pick one.
			dialHeader.Set("Sec-WebSocket-Protocol", sp)
		}

		dialCtx, span := otel.Tracer("homenavi/websocket").Start(r.Context(), "websocket.connect", trace.WithSpanKind(trace.SpanKindClient))
		propagation.TraceContext{}.Inject(dialCtx, propagation.HeaderCarrier(dialHeader))
		backendConn, backendResp, err := websocket.DefaultDialer.DialContext(dialCtx, u.String(), dialHeader)
		if err != nil {
			span.SetStatus(codes.Error, "websocket upstream connection failed")
			span.End()
			slog.Error("websocket backend dial failed", "upstream", u.String(), "error", err)
			status := http.StatusBadGateway
			if backendResp != nil && backendResp.StatusCode != 0 {
				status = backendResp.StatusCode
			}
			http.Error(w, http.StatusText(status), status)
			return
		}
		span.End()
		defer backendConn.Close()

		selectedProto := ""
		if backendResp != nil {
			selectedProto = backendResp.Header.Get("Sec-WebSocket-Protocol")
		}

		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true }, // upstream auth already handled elsewhere
		}
		if selectedProto != "" {
			upgrader.Subprotocols = []string{selectedProto}
		}

		clientConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("websocket upgrade failed", "error", err)
			return
		}
		observability.FinishHTTPUpgrade(r.Context())
		defer clientConn.Close()

		// Pipe data both directions.
		var (
			wg          sync.WaitGroup
			once        sync.Once
			closeNormal = func() {
				_ = backendConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(100*time.Millisecond))
				_ = clientConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(100*time.Millisecond))
			}
		)

		pump := func(src, dst *websocket.Conn, dir string) {
			defer wg.Done()
			for {
				mt, msg, err := src.ReadMessage()
				if err != nil {
					if !isBenignWSClose(err) {
						slog.Debug("websocket proxy read end", "direction", dir, "error", err)
					}
					once.Do(closeNormal)
					return
				}
				if err = dst.WriteMessage(mt, msg); err != nil {
					if !isBenignWSClose(err) {
						slog.Debug("websocket proxy write end", "direction", dir, "error", err)
					}
					once.Do(closeNormal)
					return
				}
			}
		}

		wg.Add(2)
		go pump(clientConn, backendConn, "client->backend")
		go pump(backendConn, clientConn, "backend->client")
		wg.Wait()
	}
}

// isBenignWSClose returns true for errors that represent normal or client initiated
// connection shutdowns that we don't want to log as noisy INFO/ERROR messages.
func isBenignWSClose(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		switch closeErr.Code {
		case websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived:
			return true
		}
	}
	// Fallback textual heuristics for abrupt FIN without close frame (library surfaces 1006 / unexpected EOF)
	es := err.Error()
	if strings.Contains(es, "close 1006") || strings.Contains(es, "unexpected EOF") || strings.Contains(es, "use of closed network connection") {
		return true
	}
	return false
}
