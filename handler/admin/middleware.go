package admin

import (
	"net/http"
	"strings"

	"kbfirmware/db"
)

// AuthMiddleware checks for a valid admin token.
// Accepts token via Authorization: Bearer header or ?token= query param.
// If staticToken is set, it works as a dev bypass (no DB lookup needed).
// Otherwise, the token is validated against the admin_session table.
// On failure, redirects to /admin/login.
func AuthMiddleware(staticToken string, database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ""

			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}
			if token == "" {
				token = r.URL.Query().Get("token")
			}

			if token != "" {
				// Static dev bypass
				if staticToken != "" && token == staticToken {
					next.ServeHTTP(w, r)
					return
				}
				// DB session check
				if ok, err := database.VerifySession(token); err == nil && ok {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		})
	}
}
