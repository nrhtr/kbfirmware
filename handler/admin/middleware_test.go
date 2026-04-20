package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"kbfirmware/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func applyMiddleware(staticToken string, database *db.DB, token string) *httptest.ResponseRecorder {
	mw := AuthMiddleware(staticToken, database)(okHandler)
	r := httptest.NewRequest(http.MethodGet, "/admin/manage?token="+token, nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)
	return w
}

func TestAuthMiddleware_StaticTokenBypass(t *testing.T) {
	database := openTestDB(t)
	w := applyMiddleware("devtoken", database, "devtoken")
	if w.Code != http.StatusOK {
		t.Errorf("static bypass: got %d want 200", w.Code)
	}
}

func TestAuthMiddleware_WrongStaticToken(t *testing.T) {
	database := openTestDB(t)
	w := applyMiddleware("devtoken", database, "wrongtoken")
	if w.Code != http.StatusSeeOther {
		t.Errorf("wrong static token: got %d want 303", w.Code)
	}
}

func TestAuthMiddleware_NoToken_Redirects(t *testing.T) {
	database := openTestDB(t)
	mw := AuthMiddleware("", database)(okHandler)
	r := httptest.NewRequest(http.MethodGet, "/admin/manage", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Errorf("no token: got %d want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/admin/login" {
		t.Errorf("redirect location: got %q want /admin/login", loc)
	}
}

func TestAuthMiddleware_ValidDBSession(t *testing.T) {
	database := openTestDB(t)
	if err := database.CreateSession("valid-session"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	w := applyMiddleware("", database, "valid-session")
	if w.Code != http.StatusOK {
		t.Errorf("valid DB session: got %d want 200", w.Code)
	}
}

func TestAuthMiddleware_ExpiredDBSession(t *testing.T) {
	database := openTestDB(t)
	database.Exec(`INSERT INTO admin_session (token, expires_at) VALUES ('expsess', unixepoch() - 1)`)
	w := applyMiddleware("", database, "expsess")
	if w.Code != http.StatusSeeOther {
		t.Errorf("expired session: got %d want 303", w.Code)
	}
}

func TestAuthMiddleware_BearerHeader(t *testing.T) {
	database := openTestDB(t)
	database.CreateSession("bearer-token")

	mw := AuthMiddleware("", database)(okHandler)
	r := httptest.NewRequest(http.MethodGet, "/admin/manage", nil)
	r.Header.Set("Authorization", "Bearer bearer-token")
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("bearer token: got %d want 200", w.Code)
	}
}
