package admin

import (
	"html/template"
	"log"
	"net/http"

	"kbfirmware/db"
)

// AnalyticsHandler handles GET /admin/analytics.
type AnalyticsHandler struct {
	DB   *db.DB
	Tmpl *template.Template
}

type analyticsData struct {
	Daily     []db.DailyStat
	Downloads []db.DownloadStat
	Token     string
}

func (h *AnalyticsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	daily, downloads, err := h.DB.AnalyticsOverview()
	if err != nil {
		log.Printf("analytics: AnalyticsOverview: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := analyticsData{
		Daily:     daily,
		Downloads: downloads,
		Token:     r.URL.Query().Get("token"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Tmpl.ExecuteTemplate(w, "analytics.html", data); err != nil {
		log.Printf("analytics: template error: %v", err)
	}
}
