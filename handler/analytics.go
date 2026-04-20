package handler

import (
	"crypto/sha256"
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

func (h *AnalyticsHandler) RecordVisit(w http.ResponseWriter, r *http.Request) {
	ipHash := hashIP(realIP(r), h.Salt)
	country := r.Header.Get("CF-IPCountry")
	if err := h.DB.RecordAnalyticsEvent("visit", nil, ipHash, country); err != nil {
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
	ipHash := hashIP(realIP(r), h.Salt)
	country := r.Header.Get("CF-IPCountry")
	if err := h.DB.RecordAnalyticsEvent("download", &fileID, ipHash, country); err != nil {
		log.Printf("analytics: record download file %d: %v", fileID, err)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func hashIP(ip, salt string) string {
	h := sha256.Sum256([]byte(ip + salt))
	return fmt.Sprintf("%x", h[:8]) // 16 hex chars, enough for deduplication
}
