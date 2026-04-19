package search

// Entry is the JSON shape embedded in the index page for client-side search.
type Entry struct {
	ID           int64    `json:"id"`
	PCBName      string   `json:"pcb_name"`
	PCBRevision  string   `json:"pcb_revision"`
	PCBDesigner  string   `json:"pcb_designer"`
	FirmwareName string   `json:"firmware_name"`
	SourceURL    string   `json:"source_url"`
	Notes        string   `json:"notes"`
	Tags         []string `json:"tags"`
	Files        []File   `json:"files"`
}

// File is the JSON shape for a firmware file within a search Entry.
type File struct {
	ID        int64  `json:"id"`
	FileTag   string `json:"file_tag"`
	Filename  string `json:"filename"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}
