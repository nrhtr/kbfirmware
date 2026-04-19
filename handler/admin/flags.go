package admin

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"kbfirmware/db"
	"kbfirmware/email"
)

// FlagsHandler handles GET /admin/flags — lists open flags.
type FlagsHandler struct {
	DB   *db.DB
	Tmpl *template.Template
}

type flagsData struct {
	Flags []db.Flag
	Token string
}

func (h *FlagsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flags, err := h.DB.OpenFlags()
	if err != nil {
		log.Printf("flags: OpenFlags: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := flagsData{
		Flags: flags,
		Token: r.URL.Query().Get("token"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Tmpl.ExecuteTemplate(w, "flags.html", data); err != nil {
		log.Printf("flags: template error: %v", err)
	}
}

// FlagsJSONHandler handles GET /admin/flags.json — returns open flags as JSON.
type FlagsJSONHandler struct {
	DB *db.DB
}

func (h *FlagsJSONHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flags, err := h.DB.OpenFlags()
	if err != nil {
		log.Printf("flags json: OpenFlags: %v", err)
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	// Normalise nil to empty slice so JSON is [] not null
	if flags == nil {
		flags = []db.Flag{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(flags)
}

// SendDigestHandler handles POST /admin/send-digest — immediately sends the flag digest email.
// Useful for testing SMTP config and for manual out-of-schedule sends.
type SendDigestHandler struct {
	DB          *db.DB
	EmailConfig email.Config
}

func (h *SendDigestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flags, err := h.DB.OpenFlags()
	if err != nil {
		log.Printf("send-digest: OpenFlags: %v", err)
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if h.EmailConfig.To == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "SMTP_TO is not configured"})
		return
	}

	if err := email.SendDigest(h.EmailConfig, flags); err != nil {
		log.Printf("send-digest: SendDigest: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	log.Printf("send-digest: sent digest with %d flag(s) (manual trigger)", len(flags))
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "flags_included": len(flags)})
}

// ResolveFlagHandler handles POST /admin/flag/{id}/resolve — resolves a flag.
type ResolveFlagHandler struct {
	DB *db.DB
}

func (h *ResolveFlagHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	notes := strings.TrimSpace(r.FormValue("notes"))

	if err := h.DB.ResolveFlag(id, notes); err != nil {
		log.Printf("resolve flag: ResolveFlag(%d): %v", id, err)
		http.Error(w, "failed to resolve flag", http.StatusInternalServerError)
		return
	}

	token := r.URL.Query().Get("token")
	redirect := "/admin/flags"
	if token != "" {
		redirect = "/admin/flags?token=" + token
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
