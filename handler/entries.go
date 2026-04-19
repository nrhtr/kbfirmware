package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"kbfirmware/db"
	"kbfirmware/search"
)

// EntriesJSONHandler serves GET /api/entries.json for client-side search.
type EntriesJSONHandler struct {
	DB *db.DB
}

func (h *EntriesJSONHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	version, err := h.DB.ContentVersion()
	if err != nil {
		log.Printf("entries json: ContentVersion: %v", err)
	}

	etag := fmt.Sprintf(`"v%d"`, version)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=300, stale-while-revalidate=30")

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	entries, err := h.DB.AllEntries()
	if err != nil {
		log.Printf("entries json: AllEntries: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	payload := make([]search.Entry, 0, len(entries))
	for _, e := range entries {
		se := search.Entry{
			ID:           e.ID,
			PCBName:      e.PCBName,
			PCBRevision:  e.PCBRevision,
			PCBDesigner:  e.PCBDesigner,
			FirmwareName: e.FirmwareName,
			SourceURL:    e.SourceURL,
			Notes:        e.Notes,
			Tags:         e.Tags,
			Files:        make([]search.File, 0, len(e.Files)),
		}
		for _, f := range e.Files {
			se.Files = append(se.Files, search.File{
				ID:       f.ID,
				FileTag:  f.FileTag,
				Filename: f.Filename,
				SHA256:   f.SHA256,
			})
		}
		payload = append(payload, se)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("entries json: encode: %v", err)
	}
}
