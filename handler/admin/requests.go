package admin

import (
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"kbfirmware/db"
)

// RequestsHandler handles GET /admin/requests.
type RequestsHandler struct {
	DB   *db.DB
	Tmpl *template.Template
}

type requestsData struct {
	Requests  []db.FirmwareRequest
	Token     string
	ActiveNav string
}

func (h *RequestsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqs, err := h.DB.OpenRequests()
	if err != nil {
		log.Printf("requests: OpenRequests: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Tmpl.ExecuteTemplate(w, "requests.html", requestsData{
		Requests:  reqs,
		Token:     r.URL.Query().Get("token"),
		ActiveNav: "requests",
	}); err != nil {
		log.Printf("requests: template: %v", err)
	}
}

// ResolveRequestHandler handles POST /admin/request/{id}/resolve.
type ResolveRequestHandler struct {
	DB *db.DB
}

func (h *ResolveRequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.DB.ResolveRequest(id); err != nil {
		log.Printf("resolve request: ResolveRequest(%d): %v", id, err)
		http.Error(w, "failed to resolve request", http.StatusInternalServerError)
		return
	}

	token := r.URL.Query().Get("token")
	redirect := "/admin/requests"
	if token != "" {
		redirect = "/admin/requests?token=" + token
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
