package admin

import (
	"net/http"
	"strings"
)

// AuthMiddleware returns middleware that checks for a valid admin token.
// Accepts token via:
//   - Authorization: Bearer <token> header
//   - ?token=<token> query parameter (for browser bookmarks)
func AuthMiddleware(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ""

			// Check Authorization header first
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}

			// Fall back to query param
			if token == "" {
				token = r.URL.Query().Get("token")
			}

			if adminToken == "" || token != adminToken {
				w.Header().Set("WWW-Authenticate", `Bearer realm="kbfirmware admin"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
