package handler

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"kbfirmware/db"
)

// EntryHandler serves GET /entry/{id} — SSR page for a single firmware entry.
type EntryHandler struct {
	DB      *db.DB
	Tmpl    *template.Template
	SiteURL string
}

func (h *EntryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	entry, err := h.DB.EntryByID(id)
	if err != nil {
		log.Printf("entry: EntryByID(%d): %v", id, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if entry == nil {
		http.NotFound(w, r)
		return
	}

	canonicalQ := entry.PCBName
	if entry.PCBRevision != "" {
		canonicalQ += " " + entry.PCBRevision
	}

	data := struct {
		Entry     *db.FirmwareEntry
		Canonical string // canonical URL back to search
	}{
		Entry:     entry,
		Canonical: fmt.Sprintf("/?q=%s", template.URLQueryEscaper(canonicalQ)),
	}

	w.Header().Set("Cache-Control", "public, max-age=300, stale-while-revalidate=30")
	if err := h.Tmpl.ExecuteTemplate(w, "entry.html", data); err != nil {
		log.Printf("entry: template: %v", err)
	}
}

// SitemapHandler serves GET /sitemap.xml listing all entry URLs.
type SitemapHandler struct {
	DB      *db.DB
	SiteURL string
}

func (h *SitemapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	entries, err := h.DB.AllEntries()
	if err != nil {
		log.Printf("sitemap: AllEntries: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")
	fmt.Fprintf(w, `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`+"\n")
	fmt.Fprintf(w, "  <url><loc>%s/</loc></url>\n", h.SiteURL)
	for _, e := range entries {
		fmt.Fprintf(w, "  <url><loc>%s/entry/%d</loc></url>\n", h.SiteURL, e.ID)
	}
	fmt.Fprintf(w, `</urlset>`)
}
