package admin

import (
	"crypto/sha256"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"kbfirmware/db"
)

const maxUploadSize = 64 << 20 // 64MB

// UploadFormHandler handles GET /admin/upload — shows the upload form.
type UploadFormHandler struct {
	DB   *db.DB
	Tmpl *template.Template
}

type uploadFormData struct {
	PCBs      []db.PCB
	Tags      []string
	Token     string
	Error     string
	ActiveNav string
}

func (h *UploadFormHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pcbs, err := h.DB.AllPCBs()
	if err != nil {
		log.Printf("upload form: AllPCBs: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tags, err := h.DB.AllTags()
	if err != nil {
		log.Printf("upload form: AllTags: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := uploadFormData{
		PCBs:      pcbs,
		Tags:      tags,
		Token:     r.URL.Query().Get("token"),
		ActiveNav: "upload",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Tmpl.ExecuteTemplate(w, "upload.html", data); err != nil {
		log.Printf("upload form: template error: %v", err)
	}
}

// UploadHandler handles POST /admin/upload — processes the upload form.
type UploadHandler struct {
	DB   *db.DB
	Tmpl *template.Template
}

func (h *UploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	redirectBase := "/admin/manage"
	if token != "" {
		redirectBase = "/admin/manage?token=" + token
	}

	// Determine PCB ID
	var pcbID int64
	pcbIDStr := r.FormValue("pcb_id")
	if pcbIDStr == "new" {
		pcbName := strings.TrimSpace(r.FormValue("pcb_name"))
		if pcbName == "" {
			h.renderError(w, r, "PCB name is required for new PCB")
			return
		}
		pcbRevision := strings.TrimSpace(r.FormValue("pcb_revision"))
		pcbDesigner := strings.TrimSpace(r.FormValue("pcb_designer"))
		pcbNotes := strings.TrimSpace(r.FormValue("pcb_notes"))

		id, err := h.DB.InsertPCB(pcbName, pcbRevision, pcbDesigner, pcbNotes)
		if err != nil {
			log.Printf("upload: InsertPCB: %v", err)
			h.renderError(w, r, "failed to create PCB: "+err.Error())
			return
		}
		pcbID = id
	} else {
		id, err := strconv.ParseInt(pcbIDStr, 10, 64)
		if err != nil {
			h.renderError(w, r, "invalid PCB selection")
			return
		}
		pcbID = id
	}

	// Firmware fields
	firmwareName := strings.TrimSpace(r.FormValue("firmware_name"))
	if firmwareName == "" {
		h.renderError(w, r, "firmware name is required")
		return
	}
	sourceURL := strings.TrimSpace(r.FormValue("source_url"))
	firmwareNotes := strings.TrimSpace(r.FormValue("firmware_notes"))

	// Tags: comma-separated
	rawTags := r.FormValue("tags")
	var tags []string
	for _, t := range strings.Split(rawTags, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}

	// Insert firmware entry
	entryID, err := h.DB.InsertEntry(pcbID, firmwareName, sourceURL, firmwareNotes, tags)
	if err != nil {
		log.Printf("upload: InsertEntry: %v", err)
		h.renderError(w, r, "failed to create firmware entry: "+err.Error())
		return
	}

	// Process up to 5 file uploads
	for i := 0; i < 5; i++ {
		fileTagKey := fmt.Sprintf("file_tag_%d", i)
		fileKey := fmt.Sprintf("file_%d", i)

		fileTag := strings.TrimSpace(r.FormValue(fileTagKey))

		file, header, err := r.FormFile(fileKey)
		if err != nil {
			// No file uploaded for this slot — skip
			continue
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			log.Printf("upload: read file_%d: %v", i, err)
			continue
		}

		if len(data) == 0 {
			continue
		}

		// Compute SHA256
		hash := sha256.Sum256(data)
		sha256Hex := fmt.Sprintf("%x", hash)

		// Detect MIME type
		mimeType := http.DetectContentType(data)

		filename := header.Filename
		if filename == "" {
			filename = fmt.Sprintf("file_%d", i)
		}

		if fileTag == "" {
			fileTag = filename
		}

		_, err = h.DB.InsertFile(entryID, fileTag, filename, mimeType, sha256Hex, int64(len(data)), data)
		if err != nil {
			log.Printf("upload: InsertFile slot %d: %v", i, err)
		}
	}

	http.Redirect(w, r, redirectBase, http.StatusSeeOther)
}

func (h *UploadHandler) renderError(w http.ResponseWriter, r *http.Request, errMsg string) {
	pcbs, _ := h.DB.AllPCBs()
	tags, _ := h.DB.AllTags()
	data := uploadFormData{
		PCBs:      pcbs,
		Tags:      tags,
		Token:     r.URL.Query().Get("token"),
		Error:     errMsg,
		ActiveNav: "upload",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	if err := h.Tmpl.ExecuteTemplate(w, "upload.html", data); err != nil {
		log.Printf("upload: error template: %v", err)
	}
}
