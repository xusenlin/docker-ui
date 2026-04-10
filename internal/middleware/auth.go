package middleware

import (
	"crypto/subtle"
	"net/http"
)

func AuthMiddleware(user, pass string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="Docker UI"`)
				http.Error(w, "authorization required", http.StatusUnauthorized)
				return
			}

			if subtle.ConstantTimeCompare([]byte(u), []byte(user)) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), []byte(pass)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Docker UI"`)
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
