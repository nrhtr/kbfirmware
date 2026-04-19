package handler

import (
	"html/template"
	"log"
	"net/http"
)

// IndexHandler renders the main search page shell.
// Entry data is loaded client-side from /api/entries.json.
type IndexHandler struct {
	Tmpl *template.Template
}

func (h *IndexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Tmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
		log.Printf("index: template error: %v", err)
	}
}
