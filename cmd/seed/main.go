// cmd/seed imports VIA-compatible firmware files from the-via/firmware into the kbfirmware DB.
//
// Usage:
//
//	DB_PATH=kbfirmware.db go run ./cmd/seed/
//
// Safe to run multiple times — already-imported files are skipped by filename.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"kbfirmware/db"
)

const (
	repoTreeAPI = "https://api.github.com/repos/the-via/firmware/git/trees/master?recursive=1"
	rawBase     = "https://raw.githubusercontent.com/the-via/firmware/master/"
	sourceRef   = "https://github.com/the-via/firmware"
)

type treeResponse struct {
	Tree     []treeEntry `json:"tree"`
	Truncated bool       `json:"truncated"`
}

type treeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "kbfirmware.db"
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	log.Println("Fetching file list from GitHub...")
	tree, err := fetchTree()
	if err != nil {
		log.Fatalf("fetch tree: %v", err)
	}
	if tree.Truncated {
		log.Println("WARNING: GitHub tree response was truncated; some files may be missing")
	}

	var files []treeEntry
	for _, e := range tree.Tree {
		if e.Type == "blob" && isFirmwareFile(e.Path) {
			files = append(files, e)
		}
	}
	log.Printf("Found %d firmware files to import", len(files))

	ok, skipped, failed := 0, 0, 0
	for i, f := range files {
		result, err := seedFile(database, f.Path, i+1, len(files))
		switch result {
		case "skipped":
			skipped++
		case "ok":
			ok++
		default:
			failed++
			log.Printf("[%d/%d] FAIL %s: %v", i+1, len(files), f.Path, err)
		}
		// Polite delay between downloads
		time.Sleep(80 * time.Millisecond)
	}

	log.Printf("Done. imported=%d skipped=%d failed=%d", ok, skipped, failed)
}

func isFirmwareFile(path string) bool {
	// Only top-level files (no subdirs) matching the VIA naming convention
	if strings.Contains(path, "/") {
		return false
	}
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, "_via.hex") ||
		strings.HasSuffix(lower, "_via.bin") ||
		strings.HasSuffix(lower, "_via.uf2")
}

func fetchTree() (*treeResponse, error) {
	req, err := http.NewRequest("GET", repoTreeAPI, nil)
	if err != nil {
		return nil, err
	}
	// GitHub recommends a User-Agent header
	req.Header.Set("User-Agent", "kbfirmware-seed/1.0")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}

	var tree treeResponse
	return &tree, json.NewDecoder(resp.Body).Decode(&tree)
}

func seedFile(database *db.DB, filename string, idx, total int) (string, error) {
	exists, err := database.FileExistsByFilename(filename)
	if err != nil {
		return "error", err
	}
	if exists {
		log.Printf("[%d/%d] skip (already imported): %s", idx, total, filename)
		return "skipped", nil
	}

	info := parseFilename(filename)

	data, err := downloadFile(rawBase + filename)
	if err != nil {
		return "error", fmt.Errorf("download: %w", err)
	}

	sum := sha256.Sum256(data)
	sha256hex := hex.EncodeToString(sum[:])

	pcbID, err := database.UpsertPCB(info.PCBName, info.PCBRevision, info.Designer, "")
	if err != nil {
		return "error", fmt.Errorf("upsert pcb: %w", err)
	}

	entryID, err := database.InsertEntry(pcbID, info.FirmwareName, rawBase+filename,
		"Imported from github.com/the-via/firmware", []string{"qmk", "via"})
	if err != nil {
		return "error", fmt.Errorf("insert entry: %w", err)
	}

	_, err = database.InsertFile(entryID, "firmware", filename,
		"application/octet-stream", sha256hex, int64(len(data)), data)
	if err != nil {
		return "error", fmt.Errorf("insert file: %w", err)
	}

	log.Printf("[%d/%d] imported: %s → %q by %q", idx, total, filename, info.PCBName, info.Designer)
	return "ok", nil
}

type fileInfo struct {
	PCBName      string
	PCBRevision  string
	Designer     string
	FirmwareName string
}

// parseFilename extracts display metadata from a VIA firmware filename.
//
// Filename pattern: {vendor}_{board}[_{variant}]_via.{ext}
// The vendor (first segment) becomes the designer.
// Everything after the vendor prefix becomes the PCB name.
// Any variant detail (hotswap, solder, rev, ANSI, ISO, etc.) is folded into the PCB name
// so that distinct variants appear as separate searchable entries.
//
// Examples:
//
//	cannonkeys_bakeneko60_iso_hs_via.bin → designer=Cannonkeys, pcb=Bakeneko60 ISO HS
//	coseyfannitutti_discipline_via.hex   → designer=Coseyfannitutti, pcb=Discipline
//	4pplet_waffling60_rev_d_ansi_via.bin → designer=4pplet, pcb=Waffling60 Rev D ANSI
func parseFilename(filename string) fileInfo {
	// Strip _via.{ext} suffix
	stem := filename
	for _, sfx := range []string{"_via.hex", "_via.bin", "_via.uf2"} {
		if strings.HasSuffix(strings.ToLower(stem), sfx) {
			stem = stem[:len(stem)-len(sfx)]
			break
		}
	}

	// Split into vendor + rest at first underscore
	vendor, rest, _ := strings.Cut(stem, "_")
	if rest == "" {
		// Single-segment filename — use as both
		return fileInfo{
			PCBName:      prettify(stem),
			Designer:     prettify(vendor),
			FirmwareName: "VIA Firmware",
		}
	}

	pcbName := prettify(rest)
	firmwareName := "VIA Firmware"

	return fileInfo{
		PCBName:      pcbName,
		PCBRevision:  "",
		Designer:     prettify(vendor),
		FirmwareName: firmwareName,
	}
}

// prettify converts a snake_case identifier to a display string with known acronyms preserved.
func prettify(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if upper, ok := acronyms[strings.ToLower(p)]; ok {
			parts[i] = upper
		} else if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

var acronyms = map[string]string{
	"rgb":    "RGB",
	"ansi":   "ANSI",
	"iso":    "ISO",
	"jis":    "JIS",
	"hs":     "HS",
	"tkl":    "TKL",
	"via":    "VIA",
	"pcb":    "PCB",
	"rp2040": "RP2040",
	"kb2040": "KB2040",
	"ce":     "CE",
	"lm":     "LM",
	"le":     "LE",
	"oe":     "OE",
	"avr":    "AVR",
	"arm":    "ARM",
	"usb":    "USB",
	"led":    "LED",
	"ble":    "BLE",
	"oled":   "OLED",
}

func downloadFile(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "kbfirmware-seed/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
