package db

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

var (
	zstdEncoder, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	zstdDecoder, _ = zstd.NewReader(nil)
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps sql.DB with app-specific methods.
type DB struct {
	*sql.DB
}

// PCB represents a keyboard PCB record.
type PCB struct {
	ID        int64
	Name      string
	Revision  string
	Designer  string
	Notes     string
	CreatedAt int64
}

// FirmwareEntry represents a firmware entry with joined PCB and file data.
type FirmwareEntry struct {
	ID           int64
	PCBID        int64
	PCBName      string
	PCBRevision  string
	PCBDesigner  string
	FirmwareName string
	SourceURL    string
	Notes        string
	Tags         []string
	Files        []FirmwareFile
	CreatedAt    int64
	UpdatedAt    int64
}

// FirmwareFile represents a file attached to a firmware entry.
type FirmwareFile struct {
	ID              int64
	FirmwareEntryID int64
	FileTag         string
	Filename        string
	MimeType        string
	SHA256          string
	SizeBytes       int64
	UploadedAt      int64
	// Data not included in listing structs — only fetched for download
}

// Flag represents a user-submitted report/flag on a firmware entry.
type Flag struct {
	ID              int64
	FirmwareEntryID int64
	FirmwareName    string
	PCBName         string
	Reason          string
	ReporterIP      string
	CreatedAt       int64
	Resolved        bool
	ResolutionNotes string
}

// ContentVersion returns a monotonically increasing integer that changes
// whenever entries, files, or PCBs are mutated. Used as an ETag for /api/entries.json.
func (db *DB) ContentVersion() (int64, error) {
	var v int64
	err := db.QueryRow(`PRAGMA user_version`).Scan(&v)
	return v, err
}

// bumpContentVersion increments the SQLite user_version by 1.
// Errors are silently ignored — the worst case is a stale ETag.
func (db *DB) bumpContentVersion() {
	var v int64
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return
	}
	db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v+1))
}

// Open opens the SQLite database at path and runs any pending goose migrations.
// WAL mode and foreign keys are set via DSN so they apply to every connection.
func Open(path string) (*DB, error) {
	// _pragma params are applied by modernc.org/sqlite on every new connection.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return nil, fmt.Errorf("goose up: %w", err)
	}

	return &DB{sqlDB}, nil
}

// AllEntries returns all firmware entries with full PCB, tag, and file data.
func (db *DB) AllEntries() ([]FirmwareEntry, error) {
	// First, fetch all entries with PCB data
	rows, err := db.Query(`
		SELECT
			fe.id, fe.pcb_id, p.name, p.revision, p.designer,
			fe.firmware_name, fe.source_url, fe.notes,
			fe.created_at, fe.updated_at
		FROM firmware_entry fe
		JOIN pcb p ON p.id = fe.pcb_id
		ORDER BY p.name ASC, fe.firmware_name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}
	defer rows.Close()

	entries := []FirmwareEntry{}
	entryIndexByID := map[int64]int{}

	for rows.Next() {
		var e FirmwareEntry
		if err := rows.Scan(
			&e.ID, &e.PCBID, &e.PCBName, &e.PCBRevision, &e.PCBDesigner,
			&e.FirmwareName, &e.SourceURL, &e.Notes,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		e.Tags = []string{}
		e.Files = []FirmwareFile{}
		entryIndexByID[e.ID] = len(entries)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return entries, nil
	}

	// Fetch tags for all entries
	tagRows, err := db.Query(`
		SELECT fet.firmware_entry_id, t.name
		FROM firmware_entry_tag fet
		JOIN tag t ON t.id = fet.tag_id
		ORDER BY t.name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer tagRows.Close()

	for tagRows.Next() {
		var entryID int64
		var tagName string
		if err := tagRows.Scan(&entryID, &tagName); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		if idx, ok := entryIndexByID[entryID]; ok {
			entries[idx].Tags = append(entries[idx].Tags, tagName)
		}
	}
	if err := tagRows.Err(); err != nil {
		return nil, err
	}

	// Fetch files for all entries
	fileRows, err := db.Query(`
		SELECT id, firmware_entry_id, file_tag, filename, mime_type, sha256, size_bytes, uploaded_at
		FROM firmware_file
		ORDER BY uploaded_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query files: %w", err)
	}
	defer fileRows.Close()

	for fileRows.Next() {
		var f FirmwareFile
		if err := fileRows.Scan(
			&f.ID, &f.FirmwareEntryID, &f.FileTag, &f.Filename,
			&f.MimeType, &f.SHA256, &f.SizeBytes, &f.UploadedAt,
		); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		if idx, ok := entryIndexByID[f.FirmwareEntryID]; ok {
			entries[idx].Files = append(entries[idx].Files, f)
		}
	}
	if err := fileRows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

// EntryByID returns a single firmware entry by ID, fully populated.
func (db *DB) EntryByID(id int64) (*FirmwareEntry, error) {
	var e FirmwareEntry
	err := db.QueryRow(`
		SELECT
			fe.id, fe.pcb_id, p.name, p.revision, p.designer,
			fe.firmware_name, fe.source_url, fe.notes,
			fe.created_at, fe.updated_at
		FROM firmware_entry fe
		JOIN pcb p ON p.id = fe.pcb_id
		WHERE fe.id = ?
	`, id).Scan(
		&e.ID, &e.PCBID, &e.PCBName, &e.PCBRevision, &e.PCBDesigner,
		&e.FirmwareName, &e.SourceURL, &e.Notes,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query entry: %w", err)
	}

	e.Tags = []string{}
	e.Files = []FirmwareFile{}

	// Fetch tags
	tagRows, err := db.Query(`
		SELECT t.name FROM firmware_entry_tag fet
		JOIN tag t ON t.id = fet.tag_id
		WHERE fet.firmware_entry_id = ?
		ORDER BY t.name ASC
	`, id)
	if err != nil {
		return nil, fmt.Errorf("query entry tags: %w", err)
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var name string
		if err := tagRows.Scan(&name); err != nil {
			return nil, err
		}
		e.Tags = append(e.Tags, name)
	}

	// Fetch files
	fileRows, err := db.Query(`
		SELECT id, firmware_entry_id, file_tag, filename, mime_type, sha256, size_bytes, uploaded_at
		FROM firmware_file
		WHERE firmware_entry_id = ?
		ORDER BY uploaded_at ASC
	`, id)
	if err != nil {
		return nil, fmt.Errorf("query entry files: %w", err)
	}
	defer fileRows.Close()
	for fileRows.Next() {
		var f FirmwareFile
		if err := fileRows.Scan(
			&f.ID, &f.FirmwareEntryID, &f.FileTag, &f.Filename,
			&f.MimeType, &f.SHA256, &f.SizeBytes, &f.UploadedAt,
		); err != nil {
			return nil, err
		}
		e.Files = append(e.Files, f)
	}

	return &e, nil
}

// AllPCBs returns all PCB records.
func (db *DB) AllPCBs() ([]PCB, error) {
	rows, err := db.Query(`SELECT id, name, revision, designer, notes, created_at FROM pcb ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("query pcbs: %w", err)
	}
	defer rows.Close()

	var pcbs []PCB
	for rows.Next() {
		var p PCB
		if err := rows.Scan(&p.ID, &p.Name, &p.Revision, &p.Designer, &p.Notes, &p.CreatedAt); err != nil {
			return nil, err
		}
		pcbs = append(pcbs, p)
	}
	return pcbs, rows.Err()
}

// AllTags returns a sorted list of all tag names.
func (db *DB) AllTags() ([]string, error) {
	rows, err := db.Query(`SELECT name FROM tag ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tags = append(tags, name)
	}
	return tags, rows.Err()
}

// InsertPCB inserts a new PCB record and returns its ID.
func (db *DB) InsertPCB(name, revision, designer, notes string) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO pcb (name, revision, designer, notes) VALUES (?, ?, ?, ?)`,
		name, revision, designer, notes,
	)
	if err != nil {
		return 0, fmt.Errorf("insert pcb: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	db.bumpContentVersion()
	return id, nil
}

// UpsertPCB inserts a PCB or returns the ID of an existing one with the same (name, revision).
func (db *DB) UpsertPCB(name, revision, designer, notes string) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM pcb WHERE name = ? AND revision = ?`, name, revision).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("lookup pcb: %w", err)
	}
	return db.InsertPCB(name, revision, designer, notes)
}

// UpdatePCB updates a PCB record by ID.
func (db *DB) UpdatePCB(id int64, name, revision, designer, notes string) error {
	_, err := db.Exec(
		`UPDATE pcb SET name=?, revision=?, designer=?, notes=? WHERE id=?`,
		name, revision, designer, notes, id,
	)
	if err != nil {
		return err
	}
	db.bumpContentVersion()
	return nil
}

// InsertEntry inserts a new firmware entry and its tags.
func (db *DB) InsertEntry(pcbID int64, firmwareName, sourceURL, notes string, tags []string) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO firmware_entry (pcb_id, firmware_name, source_url, notes) VALUES (?, ?, ?, ?)`,
		pcbID, firmwareName, sourceURL, notes,
	)
	if err != nil {
		return 0, fmt.Errorf("insert entry: %w", err)
	}
	entryID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err := db.setEntryTags(entryID, tags); err != nil {
		return 0, err
	}

	db.bumpContentVersion()
	return entryID, nil
}

// UpdateEntry updates an existing firmware entry and its tags.
func (db *DB) UpdateEntry(id int64, pcbID int64, firmwareName, sourceURL, notes string, tags []string) error {
	_, err := db.Exec(
		`UPDATE firmware_entry SET pcb_id=?, firmware_name=?, source_url=?, notes=?, updated_at=unixepoch() WHERE id=?`,
		pcbID, firmwareName, sourceURL, notes, id,
	)
	if err != nil {
		return fmt.Errorf("update entry: %w", err)
	}

	// Remove existing tags and re-add
	if _, err := db.Exec(`DELETE FROM firmware_entry_tag WHERE firmware_entry_id=?`, id); err != nil {
		return fmt.Errorf("clear entry tags: %w", err)
	}

	if err := db.setEntryTags(id, tags); err != nil {
		return err
	}
	db.bumpContentVersion()
	return nil
}

// setEntryTags upserts tags and creates firmware_entry_tag rows.
func (db *DB) setEntryTags(entryID int64, tags []string) error {
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		// Upsert tag
		_, err := db.Exec(`INSERT OR IGNORE INTO tag (name) VALUES (?)`, tag)
		if err != nil {
			return fmt.Errorf("upsert tag %q: %w", tag, err)
		}
		var tagID int64
		if err := db.QueryRow(`SELECT id FROM tag WHERE name=?`, tag).Scan(&tagID); err != nil {
			return fmt.Errorf("get tag id: %w", err)
		}
		_, err = db.Exec(
			`INSERT OR IGNORE INTO firmware_entry_tag (firmware_entry_id, tag_id) VALUES (?, ?)`,
			entryID, tagID,
		)
		if err != nil {
			return fmt.Errorf("insert entry tag: %w", err)
		}
	}
	return nil
}

// DeleteEntry deletes a firmware entry by ID (cascades to files and tags).
func (db *DB) DeleteEntry(id int64) error {
	_, err := db.Exec(`DELETE FROM firmware_entry WHERE id=?`, id)
	if err != nil {
		return err
	}
	db.bumpContentVersion()
	return nil
}

// InsertFile compresses and inserts a firmware file BLOB.
// sha256 and sizeBytes must reflect the original uncompressed data.
func (db *DB) InsertFile(entryID int64, fileTag, filename, mimeType, sha256 string, sizeBytes int64, data []byte) (int64, error) {
	compressed := zstdEncoder.EncodeAll(data, nil)
	res, err := db.Exec(
		`INSERT INTO firmware_file (firmware_entry_id, file_tag, filename, mime_type, sha256, size_bytes, data, compressed) VALUES (?, ?, ?, ?, ?, ?, ?, 1)`,
		entryID, fileTag, filename, mimeType, sha256, sizeBytes, compressed,
	)
	if err != nil {
		return 0, fmt.Errorf("insert file: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	db.bumpContentVersion()
	return id, nil
}

// GetFileData returns the file data and metadata for a given file ID.
// Data is decompressed transparently if the compressed flag is set.
func (db *DB) GetFileData(fileID int64) (filename, mimeType string, data []byte, err error) {
	var compressed int
	err = db.QueryRow(
		`SELECT filename, mime_type, data, compressed FROM firmware_file WHERE id=?`, fileID,
	).Scan(&filename, &mimeType, &data, &compressed)
	if err == sql.ErrNoRows {
		return "", "", nil, fmt.Errorf("file not found")
	}
	if err != nil {
		return "", "", nil, err
	}
	if compressed != 0 {
		data, err = zstdDecoder.DecodeAll(data, nil)
		if err != nil {
			return "", "", nil, fmt.Errorf("decompress file: %w", err)
		}
	}
	return filename, mimeType, data, nil
}

// DeleteFile deletes a firmware file by ID.
func (db *DB) DeleteFile(id int64) error {
	_, err := db.Exec(`DELETE FROM firmware_file WHERE id=?`, id)
	if err != nil {
		return err
	}
	db.bumpContentVersion()
	return nil
}

// FileExistsByFilename returns true if a firmware_file with this exact filename already exists.
func (db *DB) FileExistsByFilename(filename string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM firmware_file WHERE filename=?`, filename).Scan(&n)
	return n > 0, err
}

// InsertFlag inserts a user flag/report.
func (db *DB) InsertFlag(entryID int64, reason, reporterIP string) error {
	_, err := db.Exec(
		`INSERT INTO flag (firmware_entry_id, reason, reporter_ip) VALUES (?, ?, ?)`,
		entryID, reason, reporterIP,
	)
	return err
}

// HasRecentFlag returns true if this IP has flagged this entry in the last 24 hours.
func (db *DB) HasRecentFlag(entryID int64, ip string) (bool, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM flag WHERE firmware_entry_id=? AND reporter_ip=? AND created_at > (unixepoch() - 86400)`,
		entryID, ip,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// OpenFlags returns all unresolved flags with joined firmware/PCB names.
func (db *DB) OpenFlags() ([]Flag, error) {
	rows, err := db.Query(`
		SELECT
			f.id, f.firmware_entry_id, fe.firmware_name, p.name,
			f.reason, f.reporter_ip, f.created_at, f.resolved, f.resolution_notes
		FROM flag f
		JOIN firmware_entry fe ON fe.id = f.firmware_entry_id
		JOIN pcb p ON p.id = fe.pcb_id
		WHERE f.resolved = 0
		ORDER BY f.created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query flags: %w", err)
	}
	defer rows.Close()

	var flags []Flag
	for rows.Next() {
		var fl Flag
		var resolved int
		if err := rows.Scan(
			&fl.ID, &fl.FirmwareEntryID, &fl.FirmwareName, &fl.PCBName,
			&fl.Reason, &fl.ReporterIP, &fl.CreatedAt, &resolved, &fl.ResolutionNotes,
		); err != nil {
			return nil, err
		}
		fl.Resolved = resolved != 0
		flags = append(flags, fl)
	}
	return flags, rows.Err()
}

// ResolveFlag marks a flag as resolved with optional notes.
func (db *DB) ResolveFlag(id int64, notes string) error {
	_, err := db.Exec(
		`UPDATE flag SET resolved=1, resolution_notes=? WHERE id=?`,
		notes, id,
	)
	return err
}

// PendingFlagsCount returns the count of unresolved flags.
func (db *DB) PendingFlagsCount() (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM flag WHERE resolved=0`).Scan(&count)
	return count, err
}

// AnalyticsEvent holds data for a single analytics event.
type AnalyticsEvent struct {
	Type        string // "visit" or "download"
	FileID      *int64
	IPHash      string
	Country     string
	Path        string
	Referrer    string
	SearchQuery string
}

// CreateMagicLink stores a one-time login token expiring in 15 minutes.
func (db *DB) CreateMagicLink(token string) error {
	_, err := db.Exec(
		`INSERT INTO admin_magic_link (token, expires_at) VALUES (?, unixepoch() + 900)`,
		token,
	)
	return err
}

// VerifyMagicLink marks the token used and returns true if it was valid and unused.
func (db *DB) VerifyMagicLink(token string) (bool, error) {
	res, err := db.Exec(
		`UPDATE admin_magic_link SET used=1
		 WHERE token=? AND used=0 AND expires_at > unixepoch()`,
		token,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CreateSession stores a session token expiring in 24 hours.
func (db *DB) CreateSession(token string) error {
	_, err := db.Exec(
		`INSERT INTO admin_session (token, expires_at) VALUES (?, unixepoch() + 86400)`,
		token,
	)
	return err
}

// VerifySession returns true if the token exists and has not expired.
func (db *DB) VerifySession(token string) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM admin_session WHERE token=? AND expires_at > unixepoch()`,
		token,
	).Scan(&n)
	return n > 0, err
}

// FirmwareRequest represents a user-submitted request for new firmware.
type FirmwareRequest struct {
	ID          int64
	PCBName     string
	FirmwareURL string
	Notes       string
	Contact     string
	IPHash      string
	CreatedAt   int64
	Resolved    bool
}

// InsertFirmwareRequest stores a new firmware request.
func (db *DB) InsertFirmwareRequest(pcbName, firmwareURL, notes, contact, ipHash string) error {
	_, err := db.Exec(
		`INSERT INTO firmware_request (pcb_name, firmware_url, notes, contact, ip_hash) VALUES (?, ?, ?, ?, ?)`,
		pcbName, firmwareURL, notes, contact, ipHash,
	)
	return err
}

// OpenRequests returns all unresolved firmware requests.
func (db *DB) OpenRequests() ([]FirmwareRequest, error) {
	rows, err := db.Query(`
		SELECT id, pcb_name, firmware_url, notes, contact, ip_hash, created_at
		FROM firmware_request WHERE resolved=0 ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query requests: %w", err)
	}
	defer rows.Close()

	var reqs []FirmwareRequest
	for rows.Next() {
		var req FirmwareRequest
		if err := rows.Scan(&req.ID, &req.PCBName, &req.FirmwareURL, &req.Notes, &req.Contact, &req.IPHash, &req.CreatedAt); err != nil {
			return nil, err
		}
		reqs = append(reqs, req)
	}
	return reqs, rows.Err()
}

// ResolveRequest marks a firmware request as resolved.
func (db *DB) ResolveRequest(id int64) error {
	_, err := db.Exec(`UPDATE firmware_request SET resolved=1 WHERE id=?`, id)
	return err
}

// HasRecentRequest returns true if this IP submitted a request in the last 5 minutes.
func (db *DB) HasRecentRequest(ipHash string) (bool, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM firmware_request WHERE ip_hash=? AND created_at > (unixepoch() - 300)`,
		ipHash,
	).Scan(&count)
	return count > 0, err
}

// RecordAnalyticsEvent inserts a visit or download event.
func (db *DB) RecordAnalyticsEvent(e AnalyticsEvent) error {
	_, err := db.Exec(
		`INSERT INTO analytics_event (type, file_id, ip_hash, country, path, referrer, search_query)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Type, e.FileID, e.IPHash, e.Country, e.Path, e.Referrer, e.SearchQuery,
	)
	return err
}

// DailyStat holds aggregated visit counts for one day.
type DailyStat struct {
	Date    string
	Visits  int
	Unique  int
}

// DownloadStat holds download counts for one firmware file.
type DownloadStat struct {
	FileID    int64
	Filename  string
	EntryName string
	PCBName   string
	Downloads int
}

// ReferrerStat holds visit counts for one referrer domain.
type ReferrerStat struct {
	Referrer string
	Visits   int
}

// SearchStat holds visit counts for one search query on arrival.
type SearchStat struct {
	Query  string
	Visits int
}

// AnalyticsOverview returns summary stats for the admin analytics page.
func (db *DB) AnalyticsOverview() (daily []DailyStat, downloads []DownloadStat, referrers []ReferrerStat, searches []SearchStat, err error) {
	rows, err := db.Query(`
		SELECT
			date(created_at, 'unixepoch') AS day,
			COUNT(*) AS visits,
			COUNT(DISTINCT ip_hash) AS unique_visitors
		FROM analytics_event
		WHERE type = 'visit'
		  AND created_at >= unixepoch() - 30 * 86400
		GROUP BY day
		ORDER BY day DESC
	`)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("query daily stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s DailyStat
		if err := rows.Scan(&s.Date, &s.Visits, &s.Unique); err != nil {
			return nil, nil, nil, nil, err
		}
		daily = append(daily, s)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, nil, err
	}

	drows, err := db.Query(`
		SELECT
			ae.file_id,
			ff.filename,
			fe.firmware_name,
			p.name,
			COUNT(*) AS downloads
		FROM analytics_event ae
		JOIN firmware_file ff ON ff.id = ae.file_id
		JOIN firmware_entry fe ON fe.id = ff.firmware_entry_id
		JOIN pcb p ON p.id = fe.pcb_id
		WHERE ae.type = 'download'
		GROUP BY ae.file_id
		ORDER BY downloads DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("query download stats: %w", err)
	}
	defer drows.Close()
	for drows.Next() {
		var s DownloadStat
		if err := drows.Scan(&s.FileID, &s.Filename, &s.EntryName, &s.PCBName, &s.Downloads); err != nil {
			return nil, nil, nil, nil, err
		}
		downloads = append(downloads, s)
	}
	if err := drows.Err(); err != nil {
		return nil, nil, nil, nil, err
	}

	rrows, err := db.Query(`
		SELECT referrer, COUNT(*) AS visits
		FROM analytics_event
		WHERE type = 'visit' AND referrer != ''
		  AND created_at >= unixepoch() - 30 * 86400
		GROUP BY referrer
		ORDER BY visits DESC
		LIMIT 30
	`)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("query referrer stats: %w", err)
	}
	defer rrows.Close()
	for rrows.Next() {
		var s ReferrerStat
		if err := rrows.Scan(&s.Referrer, &s.Visits); err != nil {
			return nil, nil, nil, nil, err
		}
		referrers = append(referrers, s)
	}
	if err := rrows.Err(); err != nil {
		return nil, nil, nil, nil, err
	}

	srows, err := db.Query(`
		SELECT search_query, COUNT(*) AS visits
		FROM analytics_event
		WHERE type = 'visit' AND search_query != ''
		  AND created_at >= unixepoch() - 30 * 86400
		GROUP BY search_query
		ORDER BY visits DESC
		LIMIT 30
	`)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("query search stats: %w", err)
	}
	defer srows.Close()
	for srows.Next() {
		var s SearchStat
		if err := srows.Scan(&s.Query, &s.Visits); err != nil {
			return nil, nil, nil, nil, err
		}
		searches = append(searches, s)
	}
	return daily, downloads, referrers, searches, srows.Err()
}
