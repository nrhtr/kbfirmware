package admin

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kbfirmware/email"
)

// minimal templates sufficient for login handler rendering
var testTmpl = template.Must(template.New("login.html").Parse(
	`{{if .Sent}}sent{{else}}form{{end}}`,
))


func TestLoginHandler_GET_ShowsForm(t *testing.T) {
	db := openTestDB(t)
	h := &LoginHandler{
		DB: db, Tmpl: testTmpl,
		EmailConfig: email.Config{From: "f", To: "t"},
		SiteURL:     "http://localhost",
		SendEmail:   func(_ email.Config, _ string) error { return nil },
	}

	r := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("GET login: got %d want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "form") {
		t.Errorf("GET login: expected form in body, got %q", w.Body.String())
	}
}

func TestLoginHandler_POST_SendsEmailAndShowsSent(t *testing.T) {
	db := openTestDB(t)
	var capturedMsg string
	h := &LoginHandler{
		DB: db, Tmpl: testTmpl,
		EmailConfig: email.Config{From: "from@test", To: "to@test"},
		SiteURL:     "http://localhost",
		SendEmail: func(_ email.Config, msg string) error {
			capturedMsg = msg
			return nil
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/admin/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("POST login: got %d want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "sent") {
		t.Errorf("POST login: expected sent confirmation, got %q", w.Body.String())
	}
	if !strings.Contains(capturedMsg, "/admin/verify?token=") {
		t.Errorf("POST login: email missing verify link, got:\n%s", capturedMsg)
	}
	if !strings.Contains(capturedMsg, "http://localhost") {
		t.Errorf("POST login: email missing site URL, got:\n%s", capturedMsg)
	}
}

func TestLoginHandler_POST_StoresMagicLinkInDB(t *testing.T) {
	db := openTestDB(t)
	var sentToken string
	h := &LoginHandler{
		DB: db, Tmpl: testTmpl,
		EmailConfig: email.Config{From: "f", To: "t"},
		SiteURL:     "http://localhost",
		SendEmail: func(_ email.Config, msg string) error {
			// extract token from the verify URL embedded in the HTML
			const marker = "token="
			if idx := strings.Index(msg, marker); idx != -1 {
				rest := msg[idx+len(marker):]
				// token ends at next quote, angle bracket, or whitespace
				end := strings.IndexAny(rest, "\"<> \n")
				if end == -1 {
					end = len(rest)
				}
				sentToken = rest[:end]
			}
			return nil
		},
	}

	r := httptest.NewRequest(http.MethodPost, "/admin/login", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)

	if sentToken == "" {
		t.Fatal("no token extracted from email")
	}
	ok, err := db.VerifyMagicLink(sentToken)
	if err != nil || !ok {
		t.Errorf("magic link not stored in DB: ok=%v err=%v", ok, err)
	}
}

func TestLoginHandler_DevMode_Redirects(t *testing.T) {
	db := openTestDB(t)
	h := &LoginHandler{
		DB: db, Tmpl: testTmpl,
		EmailConfig: email.Config{},
		StaticToken: "devtoken",
		DevMode:     true,
		SendEmail:   func(_ email.Config, _ string) error { return nil },
	}

	r := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("dev mode: got %d want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/admin/manage?token=devtoken" {
		t.Errorf("dev mode redirect: got %q want /admin/manage?token=devtoken", loc)
	}
}

func TestLoginHandler_DevModeWithoutToken_ShowsForm(t *testing.T) {
	db := openTestDB(t)
	h := &LoginHandler{
		DB: db, Tmpl: testTmpl,
		EmailConfig: email.Config{From: "f", To: "t"},
		SiteURL:     "http://localhost",
		DevMode:     true, // DevMode set but no StaticToken — should still show form
		SendEmail:   func(_ email.Config, _ string) error { return nil },
	}

	r := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("dev mode no token: got %d want 200", w.Code)
	}
}

// --- VerifyHandler ---

func TestVerifyHandler_ValidToken_CreatesSessionAndRedirects(t *testing.T) {
	db := openTestDB(t)
	db.CreateMagicLink("ml-token")

	h := &VerifyHandler{DB: db, Tmpl: testTmpl}
	r := httptest.NewRequest(http.MethodGet, "/admin/verify?token=ml-token", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("verify valid: got %d want 303", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/admin/manage?token=") {
		t.Errorf("verify valid: redirect %q doesn't start with /admin/manage?token=", loc)
	}

	// The session token from the redirect should be valid in the DB
	sessionToken := strings.TrimPrefix(loc, "/admin/manage?token=")
	ok, err := db.VerifySession(sessionToken)
	if err != nil || !ok {
		t.Errorf("verify valid: session not stored in DB: ok=%v err=%v", ok, err)
	}
}

func TestVerifyHandler_InvalidToken_RedirectsToLogin(t *testing.T) {
	db := openTestDB(t)

	h := &VerifyHandler{DB: db, Tmpl: testTmpl}
	r := httptest.NewRequest(http.MethodGet, "/admin/verify?token=bogus", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("verify invalid: got %d want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/admin/login" {
		t.Errorf("verify invalid: redirect %q want /admin/login", loc)
	}
}

func TestVerifyHandler_NoToken_RedirectsToLogin(t *testing.T) {
	db := openTestDB(t)

	h := &VerifyHandler{DB: db, Tmpl: testTmpl}
	r := httptest.NewRequest(http.MethodGet, "/admin/verify", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("verify no token: got %d want 303", w.Code)
	}
}

func TestVerifyHandler_UsedToken_RedirectsToLogin(t *testing.T) {
	db := openTestDB(t)
	db.CreateMagicLink("one-time")
	db.VerifyMagicLink("one-time") // consume it

	h := &VerifyHandler{DB: db, Tmpl: testTmpl}
	r := httptest.NewRequest(http.MethodGet, "/admin/verify?token=one-time", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("verify used token: got %d want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/admin/login" {
		t.Errorf("verify used token: redirect %q want /admin/login", loc)
	}
}
