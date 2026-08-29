package httpapi

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const (
	allowedCORSMethods = "GET, POST, PUT, DELETE, OPTIONS"
	allowedCORSHeaders = "Authorization, Content-Type, If-Match, Idempotency-Key, X-Request-ID, X-Yujian-Checksum"
)

func corsMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
	allowed := append([]string(nil), allowedOrigins...)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := strings.TrimSpace(request.Header.Get("Origin"))
		if origin == "" {
			next.ServeHTTP(writer, request)
			return
		}

		originAllowed := slices.Contains(allowed, origin) || isSameHostOrigin(origin, request.Host)
		if !originAllowed {
			http.Error(writer, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		writer.Header().Add("Vary", "Origin")
		writer.Header().Set("Access-Control-Allow-Origin", origin)
		if request.Method == http.MethodOptions && request.Header.Get("Access-Control-Request-Method") != "" {
			writer.Header().Add("Vary", "Access-Control-Request-Method")
			writer.Header().Add("Vary", "Access-Control-Request-Headers")
			writer.Header().Set("Access-Control-Allow-Methods", allowedCORSMethods)
			writer.Header().Set("Access-Control-Allow-Headers", allowedCORSHeaders)
			writer.Header().Set("Access-Control-Max-Age", "600")
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(writer, request)
	})
}

func isSameHostOrigin(origin, requestHost string) bool {
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host != "" && strings.EqualFold(parsed.Host, requestHost)
}
