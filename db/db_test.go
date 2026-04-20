package db

import (
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// --- PCB ---

func TestUpsertPCB_NewAndExisting(t *testing.T) {
	db := openTestDB(t)

	id1, err := db.UpsertPCB("TestBoard", "v1", "Alice", "")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	id2, err := db.UpsertPCB("TestBoard", "v1", "Bob", "ignored")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if id1 != id2 {
		t.Errorf("expected same ID for duplicate (name, revision): got %d and %d", id1, id2)
	}
}

func TestUpsertPCB_DifferentRevision(t *testing.T) {
	db := openTestDB(t)

	id1, _ := db.UpsertPCB("Board", "v1", "", "")
	id2, _ := db.UpsertPCB("Board", "v2", "", "")

	if id1 == id2 {
		t.Error("different revisions should produce different IDs")
	}
}

// --- Entry ---

func TestInsertEntry_BasicRoundTrip(t *testing.T) {
	db := openTestDB(t)

	pcbID, _ := db.InsertPCB("PCB", "r1", "Designer", "")
	entryID, err := db.InsertEntry(pcbID, "QMK v1", "https://example.com", "notes", []string{"qmk", "iso"})
	if err != nil {
		t.Fatalf("InsertEntry: %v", err)
	}

	entry, err := db.EntryByID(entryID)
	if err != nil {
		t.Fatalf("EntryByID: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}

	if entry.FirmwareName != "QMK v1" {
		t.Errorf("FirmwareName: got %q want %q", entry.FirmwareName, "QMK v1")
	}
	if entry.SourceURL != "https://example.com" {
		t.Errorf("SourceURL: got %q want %q", entry.SourceURL, "https://example.com")
	}
	if len(entry.Tags) != 2 {
		t.Errorf("Tags: got %d want 2", len(entry.Tags))
	}
}

func TestEntryByID_NotFound(t *testing.T) {
	db := openTestDB(t)

	entry, err := db.EntryByID(999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry != nil {
		t.Error("expected nil for missing entry")
	}
}

func TestUpdateEntry_TagReplacement(t *testing.T) {
	db := openTestDB(t)

	pcbID, _ := db.InsertPCB("PCB", "", "", "")
	entryID, _ := db.InsertEntry(pcbID, "fw", "", "", []string{"qmk", "iso"})

	if err := db.UpdateEntry(entryID, pcbID, "fw", "", "", []string{"via"}); err != nil {
		t.Fatalf("UpdateEntry: %v", err)
	}

	entry, _ := db.EntryByID(entryID)
	if len(entry.Tags) != 1 || entry.Tags[0] != "via" {
		t.Errorf("Tags after update: got %v want [via]", entry.Tags)
	}
}

func TestAllEntries_MultipleEntries(t *testing.T) {
	db := openTestDB(t)

	pcbID, _ := db.InsertPCB("Alpha", "", "", "")
	db.InsertEntry(pcbID, "fw1", "", "", nil)
	db.InsertEntry(pcbID, "fw2", "", "", nil)

	entries, err := db.AllEntries()
	if err != nil {
		t.Fatalf("AllEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries want 2", len(entries))
	}
}

func TestAllEntries_EmptySliceNotNil(t *testing.T) {
	db := openTestDB(t)

	entries, err := db.AllEntries()
	if err != nil {
		t.Fatalf("AllEntries: %v", err)
	}
	if entries == nil {
		t.Error("expected non-nil empty slice")
	}
}

// --- Tags ---

func TestSetEntryTags_SkipsBlankAndWhitespace(t *testing.T) {
	db := openTestDB(t)

	pcbID, _ := db.InsertPCB("PCB", "", "", "")
	entryID, _ := db.InsertEntry(pcbID, "fw", "", "", []string{"  ", "", "qmk", "  via  "})

	entry, _ := db.EntryByID(entryID)
	// blank/whitespace-only tags should be skipped; "  via  " trimmed to "via"
	if len(entry.Tags) != 2 {
		t.Errorf("got tags %v, want [qmk via]", entry.Tags)
	}
}

func TestSetEntryTags_DeduplicatesAcrossEntries(t *testing.T) {
	db := openTestDB(t)

	pcbID, _ := db.InsertPCB("PCB", "", "", "")
	db.InsertEntry(pcbID, "fw1", "", "", []string{"qmk"})
	db.InsertEntry(pcbID, "fw2", "", "", []string{"qmk"})

	tags, err := db.AllTags()
	if err != nil {
		t.Fatalf("AllTags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "qmk" {
		t.Errorf("got tags %v, want [qmk]", tags)
	}
}

// --- File BLOB ---

func TestInsertGetFile_CompressionRoundTrip(t *testing.T) {
	db := openTestDB(t)

	pcbID, _ := db.InsertPCB("PCB", "", "", "")
	entryID, _ := db.InsertEntry(pcbID, "fw", "", "", nil)

	original := []byte("this is some firmware data that should be compressed and decompressed correctly")
	fileID, err := db.InsertFile(entryID, "firmware", "fw.bin", "application/octet-stream", "abc123", int64(len(original)), original)
	if err != nil {
		t.Fatalf("InsertFile: %v", err)
	}

	filename, mimeType, got, err := db.GetFileData(fileID)
	if err != nil {
		t.Fatalf("GetFileData: %v", err)
	}
	if filename != "fw.bin" {
		t.Errorf("filename: got %q want fw.bin", filename)
	}
	if mimeType != "application/octet-stream" {
		t.Errorf("mimeType: got %q want application/octet-stream", mimeType)
	}
	if string(got) != string(original) {
		t.Errorf("data mismatch after decompression")
	}
}

func TestGetFileData_NotFound(t *testing.T) {
	db := openTestDB(t)

	_, _, _, err := db.GetFileData(999)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// --- Content version ---

func TestContentVersion_BumpsOnWrite(t *testing.T) {
	db := openTestDB(t)

	v0, _ := db.ContentVersion()
	db.InsertPCB("PCB", "", "", "")
	v1, _ := db.ContentVersion()

	if v1 <= v0 {
		t.Errorf("version did not increase after write: %d -> %d", v0, v1)
	}
}

// --- Auth ---

func TestMagicLink_ValidToken(t *testing.T) {
	db := openTestDB(t)

	if err := db.CreateMagicLink("tok1"); err != nil {
		t.Fatalf("CreateMagicLink: %v", err)
	}

	ok, err := db.VerifyMagicLink("tok1")
	if err != nil || !ok {
		t.Errorf("expected valid magic link: ok=%v err=%v", ok, err)
	}
}

func TestMagicLink_UsedTwice(t *testing.T) {
	db := openTestDB(t)

	db.CreateMagicLink("tok2")
	db.VerifyMagicLink("tok2")

	ok, err := db.VerifyMagicLink("tok2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("used token should not verify again")
	}
}

func TestMagicLink_ExpiredToken(t *testing.T) {
	db := openTestDB(t)

	// Insert already-expired token
	_, err := db.Exec(`INSERT INTO admin_magic_link (token, expires_at) VALUES ('expired', unixepoch() - 1)`)
	if err != nil {
		t.Fatalf("insert expired token: %v", err)
	}

	ok, err := db.VerifyMagicLink("expired")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expired token should not verify")
	}
}

func TestMagicLink_UnknownToken(t *testing.T) {
	db := openTestDB(t)

	ok, err := db.VerifyMagicLink("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("unknown token should not verify")
	}
}

func TestSession_ValidToken(t *testing.T) {
	db := openTestDB(t)

	if err := db.CreateSession("sess1"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	ok, err := db.VerifySession("sess1")
	if err != nil || !ok {
		t.Errorf("expected valid session: ok=%v err=%v", ok, err)
	}
}

func TestSession_ExpiredToken(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(`INSERT INTO admin_session (token, expires_at) VALUES ('expsess', unixepoch() - 1)`)
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	ok, err := db.VerifySession("expsess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expired session should not verify")
	}
}

func TestSession_UnknownToken(t *testing.T) {
	db := openTestDB(t)

	ok, err := db.VerifySession("ghost")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("unknown session should not verify")
	}
}

// --- Delete cascade ---

func TestDeleteEntry_CascadesToFiles(t *testing.T) {
	db := openTestDB(t)

	pcbID, _ := db.InsertPCB("PCB", "", "", "")
	entryID, _ := db.InsertEntry(pcbID, "fw", "", "", nil)
	db.InsertFile(entryID, "tag", "file.bin", "application/octet-stream", "hash", 3, []byte("abc"))

	if err := db.DeleteEntry(entryID); err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}

	entries, _ := db.AllEntries()
	if len(entries) != 0 {
		t.Error("expected no entries after delete")
	}
}
