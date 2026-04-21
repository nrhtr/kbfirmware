package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"kbfirmware/db"
)

// RequestHandler handles GET/POST /request — public firmware contribution form.

type RequestHandler struct {
	DB     *db.DB
	Tmpl   *template.Template
	Secret string // HMAC key for timestamp signing
	Salt   string // for IP hashing
}

type requestFormData struct {
	Timestamp string
	Done      bool
}

func (h *RequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.showForm(w)
	case http.MethodPost:
		h.submit(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *RequestHandler) showForm(w http.ResponseWriter) {
	if err := h.Tmpl.ExecuteTemplate(w, "request.html", requestFormData{
		Timestamp: signedTimestamp(h.Secret),
	}); err != nil {
		log.Printf("request: template: %v", err)
	}
}

func (h *RequestHandler) submit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.showForm(w)
		return
	}

	// Timing check: reject submissions under 3 seconds
	if !checkTimestamp(h.Secret, r.FormValue("_ts"), 3) {
		h.showForm(w)
		return
	}

	pcbName := strings.TrimSpace(r.FormValue("pcb_name"))
	if pcbName == "" {
		h.showForm(w)
		return
	}

	firmwareURL := strings.TrimSpace(r.FormValue("firmware_url"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	contact := strings.TrimSpace(r.FormValue("contact"))

	if err := h.DB.InsertFirmwareRequest(pcbName, firmwareURL, notes, contact, hashIP(realIP(r), h.Salt)); err != nil {
		log.Printf("request: InsertFirmwareRequest: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.Tmpl.ExecuteTemplate(w, "request.html", requestFormData{Done: true}); err != nil {
		log.Printf("request: template done: %v", err)
	}
}

func signedTimestamp(secret string) string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	return ts + "." + hex.EncodeToString(mac.Sum(nil))[:16]
}

func checkTimestamp(secret, val string, minSeconds int64) bool {
	parts := strings.SplitN(val, ".", 2)
	if len(parts) != 2 {
		return false
	}
	ts, sig := parts[0], parts[1]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	expected := hex.EncodeToString(mac.Sum(nil))[:16]
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return false
	}

	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix()-t >= minSeconds
}
