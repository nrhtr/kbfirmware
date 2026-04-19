package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"kbfirmware/db"
)

// DownloadHandler serves firmware file BLOBs.
type DownloadHandler struct {
	DB *db.DB
}

func (h *DownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fileIDStr := chi.URLParam(r, "fileID")
	fileID, err := strconv.ParseInt(fileIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid file ID", http.StatusBadRequest)
		return
	}

	filename, mimeType, data, err := h.DB.GetFileData(fileID)
	if err != nil {
		log.Printf("download: GetFileData(%d): %v", fileID, err)
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
