package oauth

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func publicOriginForRequest(request *http.Request) (string, error) {
	host := forwardedValue(request.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(request.Host)
	}
	scheme := forwardedValue(request.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
		if request.TLS != nil {
			scheme = "https"
		}
	}
	origin, err := url.Parse(scheme + "://" + host)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" {
		return "", fmt.Errorf("invalid public origin")
	}
	return origin.String(), nil
}

func forwardedValue(value string) string {
	value, _, _ = strings.Cut(value, ",")
	return strings.TrimSpace(value)
}
