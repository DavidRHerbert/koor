package server

import "net/http"

// authMiddleware validates the Bearer token on every request.
// If token is empty (local mode), all requests pass through.
func authMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	expected := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != expected {
			writeError(w, http.StatusUnauthorized, "invalid or missing bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}
