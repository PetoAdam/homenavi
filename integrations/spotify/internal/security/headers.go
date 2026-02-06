package security

import "net/http"

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Permissions-Policy", "accelerometer=(), ambient-light-sensor=(), autoplay=(), battery=(), camera=(), clipboard-read=(), clipboard-write=(), display-capture=(), encrypted-media=(), fullscreen=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), midi=(), payment=(), picture-in-picture=(), publickey-credentials-get=(), usb=()")

		// Designed for iframe delivery: default deny, and only allow self-hosted resources.
		// NOTE: A host-side <iframe sandbox> is still required for strong isolation.
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; base-uri 'none'; object-src 'none'; "+
				"script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: https: https://i.scdn.co https://*.scdn.co; "+
				"connect-src 'self'; frame-ancestors 'self'")

		next.ServeHTTP(w, r)
	})
}
