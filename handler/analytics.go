package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"kbfirmware/db"
)

// AnalyticsHandler handles beacon POSTs for visit and download events.
type AnalyticsHandler struct {
	DB   *db.DB
	Salt string // secret used when hashing IPs
}

type visitPayload struct {
	Path     string `json:"path"`
	Referrer string `json:"referrer"`
	Search   string `json:"search"`
}

func (h *AnalyticsHandler) RecordVisit(w http.ResponseWriter, r *http.Request) {
	var p visitPayload
	json.NewDecoder(r.Body).Decode(&p) // best-effort; missing body is fine

	if err := h.DB.RecordAnalyticsEvent(db.AnalyticsEvent{
		Type:        "visit",
		IPHash:      hashIP(realIP(r), h.Salt),
		Country:     r.Header.Get("CF-IPCountry"),
		Path:        p.Path,
		Referrer:    p.Referrer,
		SearchQuery: p.Search,
	}); err != nil {
		log.Printf("analytics: record visit: %v", err)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *AnalyticsHandler) RecordDownload(w http.ResponseWriter, r *http.Request) {
	fileID, err := strconv.ParseInt(chi.URLParam(r, "fileID"), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.DB.RecordAnalyticsEvent(db.AnalyticsEvent{
		Type:    "download",
		FileID:  &fileID,
		IPHash:  hashIP(realIP(r), h.Salt),
		Country: r.Header.Get("CF-IPCountry"),
	}); err != nil {
		log.Printf("analytics: record download file %d: %v", fileID, err)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func hashIP(ip, salt string) string {
	h := sha256.Sum256([]byte(ip + salt))
	return fmt.Sprintf("%x", h[:8]) // 16 hex chars, enough for deduplication
}
