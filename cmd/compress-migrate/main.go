package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"

	"github.com/klauspost/compress/zstd"
	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "kbfirmware.db", "path to SQLite database")
	limit := flag.Int("limit", 0, "only migrate this many files (0 = all)")
	flag.Parse()

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", *dbPath))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))

	query := `SELECT id, filename, data FROM firmware_file WHERE compressed = 0`
	if *limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", *limit)
	}
	rows, err := db.Query(query)
	if err != nil {
		log.Fatalf("query: %v", err)
	}

	type row struct {
		id       int64
		filename string
		data     []byte
	}
	var files []row
	for rows.Next() {
		var f row
		if err := rows.Scan(&f.id, &f.filename, &f.data); err != nil {
			log.Fatalf("scan: %v", err)
		}
		files = append(files, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		log.Fatalf("rows: %v", err)
	}

	if len(files) == 0 {
		fmt.Println("No uncompressed files found.")
		return
	}

	fmt.Printf("Compressing %d files...\n\n", len(files))

	var totalBefore, totalAfter int64
	for _, f := range files {
		before := int64(len(f.data))
		compressed := encoder.EncodeAll(f.data, nil)
		after := int64(len(compressed))

		_, err := db.Exec(`UPDATE firmware_file SET data = ?, compressed = 1 WHERE id = ?`, compressed, f.id)
		if err != nil {
			log.Fatalf("update file %d (%s): %v", f.id, f.filename, err)
		}

		totalBefore += before
		totalAfter += after
		fmt.Printf("  [%d] %-45s  %s -> %s  (%.0f%%)\n",
			f.id, f.filename,
			humanBytes(before), humanBytes(after),
			float64(after)/float64(before)*100,
		)
	}

	saved := totalBefore - totalAfter
	ratio := float64(totalAfter) / float64(totalBefore) * 100
	fmt.Printf("\n%d files  |  %s -> %s  |  saved %s  (%.1f%% of original)\n",
		len(files),
		humanBytes(totalBefore), humanBytes(totalAfter),
		humanBytes(saved), ratio,
	)
}

func humanBytes(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
