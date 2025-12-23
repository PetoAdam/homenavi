package proxy

import (
	"api-gateway/internal/config"
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

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

func MakeRestProxyHandler(route config.RouteConfig) http.HandlerFunc {
	upstreamURL, err := url.Parse(route.Upstream)
	if err != nil {
		panic("Invalid upstream URL: " + route.Upstream)
	}
	routePrefix := strings.TrimSuffix(route.Path, "/*")
	upstreamPrefix := strings.TrimSuffix(upstreamURL.Path, "/*")
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("proxy request", "method", r.Method, "path", r.URL.Path, "upstream", upstreamURL.String())
		proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
		origDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			origDirector(req)
			ctx := chi.RouteContext(r.Context())
			path := upstreamURL.Path
			// If the route uses a wildcard (/*), preserve the suffix after the route prefix.
			// This allows proxying IDs that contain slashes (e.g. zigbee/0x...) without URL-encoding.
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
			req.URL.Path = path
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

func MakeWebSocketProxyHandler(route config.RouteConfig) http.HandlerFunc {
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

		backendConn, backendResp, err := websocket.DefaultDialer.Dial(u.String(), dialHeader)
		if err != nil {
			slog.Error("websocket backend dial failed", "upstream", u.String(), "error", err)
			status := http.StatusBadGateway
			if backendResp != nil && backendResp.StatusCode != 0 {
				status = backendResp.StatusCode
			}
			http.Error(w, http.StatusText(status), status)
			return
		}
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
		defer clientConn.Close()

		// Pipe data both directions.
		var (
			wg          sync.WaitGroup
			once        sync.Once
			closeNormal = func() {
				// Attempt graceful close frames (best effort)
				_ = backendConn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
				_ = backendConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				_ = clientConn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
				_ = clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
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
