package admin

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"strings"

	"kbfirmware/db"
	"kbfirmware/email"
)

type LoginHandler struct {
	DB          *db.DB
	Tmpl        *template.Template
	EmailConfig email.Config
	SiteURL     string
	StaticToken string
	DevMode     bool
	// SendEmail is called to deliver the magic link. Defaults to email.SendRaw;
	// override in tests to avoid shelling out to sendmail.
	SendEmail func(cfg email.Config, msg string) error
}

func (h *LoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.DevMode && h.StaticToken != "" {
		http.Redirect(w, r, "/admin/manage?token="+h.StaticToken, http.StatusSeeOther)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.showForm(w, r, "")
	case http.MethodPost:
		h.sendLink(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type loginData struct {
	Sent    bool
	Message string
}

func (h *LoginHandler) showForm(w http.ResponseWriter, r *http.Request, msg string) {
	if err := h.Tmpl.ExecuteTemplate(w, "login.html", loginData{Message: msg}); err != nil {
		log.Printf("login: template: %v", err)
	}
}

func (h *LoginHandler) sendLink(w http.ResponseWriter, r *http.Request) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tokenStr := hex.EncodeToString(token)

	if err := h.DB.CreateMagicLink(tokenStr); err != nil {
		log.Printf("login: CreateMagicLink: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	siteURL := strings.TrimRight(h.SiteURL, "/")
	link := siteURL + "/admin/verify?token=" + tokenStr

	msg := "From: " + h.EmailConfig.From + "\n" +
		"To: " + h.EmailConfig.To + "\n" +
		"Subject: kbfirmware admin login\n" +
		"Content-Type: text/plain; charset=UTF-8\n" +
		"\n" +
		"Click this link to log in to kbfirmware admin (expires in 15 minutes):\n\n" +
		link + "\n"

	send := h.SendEmail
	if send == nil {
		send = email.SendRaw
	}
	if err := send(h.EmailConfig, msg); err != nil {
		log.Printf("login: send magic link: %v", err)
		http.Error(w, "failed to send email", http.StatusInternalServerError)
		return
	}

	log.Printf("login: magic link sent to %s", h.EmailConfig.To)
	if err := h.Tmpl.ExecuteTemplate(w, "login.html", loginData{Sent: true}); err != nil {
		log.Printf("login: template: %v", err)
	}
}

type VerifyHandler struct {
	DB   *db.DB
	Tmpl *template.Template
}

func (h *VerifyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	ok, err := h.DB.VerifyMagicLink(token)
	if err != nil || !ok {
		log.Printf("verify: invalid magic link: ok=%v err=%v", ok, err)
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	sessionBytes := make([]byte, 32)
	if _, err := rand.Read(sessionBytes); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	sessionToken := hex.EncodeToString(sessionBytes)

	if err := h.DB.CreateSession(sessionToken); err != nil {
		log.Printf("verify: CreateSession: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/manage?token="+sessionToken, http.StatusSeeOther)
}
