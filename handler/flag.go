package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"kbfirmware/db"
)

// FlagHandler handles POST /flag/{entryID}
type FlagHandler struct {
	DB *db.DB
}

type flagRequest struct {
	Reason string `json:"reason"`
}

type flagResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func realIP(r *http.Request) string {
	// Check X-Forwarded-For first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take the first IP in the list
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	// Fall back to RemoteAddr, stripping port
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

func (h *FlagHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	entryIDStr := chi.URLParam(r, "entryID")
	entryID, err := strconv.ParseInt(entryIDStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, flagResponse{Error: "invalid entry ID"})
		return
	}

	ip := realIP(r)

	var req flagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, flagResponse{Error: "invalid request body"})
		return
	}

	if err := h.DB.InsertFlag(entryID, req.Reason, ip); err != nil {
		writeJSON(w, http.StatusInternalServerError, flagResponse{Error: "failed to submit report"})
		return
	}

	writeJSON(w, http.StatusOK, flagResponse{OK: true})
}
