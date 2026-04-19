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
)

// ManageHandler handles GET /admin/manage — lists all firmware entries.
type ManageHandler struct {
	DB   *db.DB
	Tmpl *template.Template
}

type manageData struct {
	Entries []db.FirmwareEntry
	Token   string
}

func (h *ManageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	entries, err := h.DB.AllEntries()
	if err != nil {
		log.Printf("manage: AllEntries: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := manageData{
		Entries: entries,
		Token:   r.URL.Query().Get("token"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Tmpl.ExecuteTemplate(w, "manage.html", data); err != nil {
		log.Printf("manage: template error: %v", err)
	}
}

// EditFormHandler handles GET /admin/entry/{id}/edit — shows the edit form.
type EditFormHandler struct {
	DB   *db.DB
	Tmpl *template.Template
}

type editFormData struct {
	Entry      *db.FirmwareEntry
	PCBs       []db.PCB
	CurrentPCB db.PCB
	PCBsJSON   template.JS
	Tags       []string
	Token      string
	Error      string
}

func (h *EditFormHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid ID", http.StatusBadRequest)
		return
	}

	entry, err := h.DB.EntryByID(id)
	if err != nil || entry == nil {
		http.Error(w, "entry not found", http.StatusNotFound)
		return
	}

	pcbs, err := h.DB.AllPCBs()
	if err != nil {
		log.Printf("edit form: AllPCBs: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	tags, err := h.DB.AllTags()
	if err != nil {
		log.Printf("edit form: AllTags: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var currentPCB db.PCB
	for _, p := range pcbs {
		if p.ID == entry.PCBID {
			currentPCB = p
			break
		}
	}

	// Minimal JSON shape for the dropdown-change JS handler.
	type pcbJSON struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Revision string `json:"revision"`
		Designer string `json:"designer"`
		Notes    string `json:"notes"`
	}
	pcbSlice := make([]pcbJSON, len(pcbs))
	for i, p := range pcbs {
		pcbSlice[i] = pcbJSON{p.ID, p.Name, p.Revision, p.Designer, p.Notes}
	}
	pcbsJSON, _ := json.Marshal(pcbSlice)

	data := editFormData{
		Entry:      entry,
		PCBs:       pcbs,
		CurrentPCB: currentPCB,
		PCBsJSON:   template.JS(pcbsJSON),
		Tags:       tags,
		Token:      r.URL.Query().Get("token"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Tmpl.ExecuteTemplate(w, "edit.html", data); err != nil {
		log.Printf("edit form: template error: %v", err)
	}
}

// EditHandler handles POST /admin/entry/{id}/edit — saves entry updates.
type EditHandler struct {
	DB *db.DB
}

func (h *EditHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	redirectBase := "/admin/manage"
	if token != "" {
		redirectBase = "/admin/manage?token=" + token
	}

	pcbIDStr := r.FormValue("pcb_id")
	pcbID, err := strconv.ParseInt(pcbIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid PCB ID", http.StatusBadRequest)
		return
	}

	firmwareName := strings.TrimSpace(r.FormValue("firmware_name"))
	if firmwareName == "" {
		http.Error(w, "firmware name is required", http.StatusBadRequest)
		return
	}

	sourceURL := strings.TrimSpace(r.FormValue("source_url"))
	notes := strings.TrimSpace(r.FormValue("firmware_notes"))

	rawTags := r.FormValue("tags")
	var tags []string
	for _, t := range strings.Split(rawTags, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	pcbName := strings.TrimSpace(r.FormValue("pcb_name"))
	if pcbName == "" {
		http.Error(w, "PCB name is required", http.StatusBadRequest)
		return
	}
	pcbRevision := strings.TrimSpace(r.FormValue("pcb_revision"))
	pcbDesigner := strings.TrimSpace(r.FormValue("pcb_designer"))
	pcbNotes := strings.TrimSpace(r.FormValue("pcb_notes"))

	if err := h.DB.UpdatePCB(pcbID, pcbName, pcbRevision, pcbDesigner, pcbNotes); err != nil {
		log.Printf("edit: UpdatePCB(%d): %v", pcbID, err)
		http.Error(w, "failed to update PCB", http.StatusInternalServerError)
		return
	}

	if err := h.DB.UpdateEntry(id, pcbID, firmwareName, sourceURL, notes, tags); err != nil {
		log.Printf("edit: UpdateEntry(%d): %v", id, err)
		http.Error(w, "failed to update entry", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, redirectBase, http.StatusSeeOther)
}

// DeleteEntryHandler handles POST /admin/entry/{id}/delete — deletes a firmware entry.
type DeleteEntryHandler struct {
	DB *db.DB
}

func (h *DeleteEntryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.DB.DeleteEntry(id); err != nil {
		log.Printf("delete entry: DeleteEntry(%d): %v", id, err)
		http.Error(w, "failed to delete entry", http.StatusInternalServerError)
		return
	}

	token := r.URL.Query().Get("token")
	redirect := "/admin/manage"
	if token != "" {
		redirect = "/admin/manage?token=" + token
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// DeleteFileHandler handles POST /admin/file/{id}/delete — deletes a single firmware file.
type DeleteFileHandler struct {
	DB *db.DB
}

func (h *DeleteFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid ID", http.StatusBadRequest)
		return
	}

	if err := h.DB.DeleteFile(id); err != nil {
		log.Printf("delete file: DeleteFile(%d): %v", id, err)
		http.Error(w, "failed to delete file", http.StatusInternalServerError)
		return
	}

	// Redirect to referrer, falling back to manage page
	ref := r.Header.Get("Referer")
	if ref == "" {
		token := r.URL.Query().Get("token")
		ref = "/admin/manage"
		if token != "" {
			ref = "/admin/manage?token=" + token
		}
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}
