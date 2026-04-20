package main

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	_ "time/tzdata" // embed IANA timezone DB so Docker image needs no tzdata package

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"kbfirmware/db"
	"kbfirmware/email"
	"kbfirmware/handler"
	adminhandler "kbfirmware/handler/admin"
)

//go:embed templates static
var embeddedFS embed.FS

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "compress-blobs":
			runCompressBlobs(os.Args[2:])
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\nSubcommands:\n  compress-blobs  compress uncompressed firmware BLOBs in-place\n", os.Args[1])
			os.Exit(1)
		}
	}

	// Read environment variables
	dbPath := getenv("DB_PATH", "kbfirmware.db")
	listenAddr := getenv("LISTEN_ADDR", ":8080")
	adminToken := os.Getenv("ADMIN_TOKEN")

	emailCfg := email.Config{
		From: getenv("EMAIL_FROM", "kbfirmware@jenga.xyz"),
		To:   getenv("EMAIL_TO", "jeremy@jenga.xyz"),
	}

	// Open database
	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Compute a short content hash over all embedded static files for cache busting.
	assetVer := staticVersion(embeddedFS)

	// Template functions
	funcMap := template.FuncMap{
		// asset returns the /static/ URL with a cache-busting version query param
		"asset": func(name string) string {
			return "/static/" + name + "?v=" + assetVer
		},
		// join concatenates a slice of strings with a separator
		"join": func(elems []string, sep string) string {
			return strings.Join(elems, sep)
		},
		// slice returns s[start:end] for use in templates
		"slice": func(s string, start, end int) string {
			if end > len(s) {
				end = len(s)
			}
			if start > len(s) {
				start = len(s)
			}
			return s[start:end]
		},
		// not negates a boolean
		"not": func(v interface{}) bool {
			if v == nil {
				return true
			}
			switch val := v.(type) {
			case bool:
				return !val
			case int:
				return val == 0
			case string:
				return val == ""
			}
			return false
		},
		// add adds two ints
		"add": func(a, b int) int {
			return a + b
		},
		// iter returns a slice of ints [0, n)
		"iter": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
	}

	// Parse all templates with custom functions
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(embeddedFS,
		"templates/*.html",
		"templates/admin/*.html",
	)
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}

	// Start daily email digest scheduler
	email.StartDailyDigest(emailCfg, database)

	// Build router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	analytics := &handler.AnalyticsHandler{DB: database, Salt: adminToken}

	// Public routes
	r.Get("/", (&handler.IndexHandler{Tmpl: tmpl}).ServeHTTP)
	r.Get("/api/entries.json", (&handler.EntriesJSONHandler{DB: database}).ServeHTTP)
	r.Get("/file/{fileID}", (&handler.DownloadHandler{DB: database}).ServeHTTP)
	r.Get("/file/{fileID}/{sha256}", (&handler.DownloadHandler{DB: database}).ServeHTTP)
	r.Post("/flag/{entryID}", (&handler.FlagHandler{DB: database}).ServeHTTP)
	r.Post("/analytics/visit", analytics.RecordVisit)
	r.Post("/analytics/download/{fileID}", analytics.RecordDownload)

	// Admin sub-router
	r.Route("/admin", func(r chi.Router) {
		r.Use(adminhandler.AuthMiddleware(adminToken))

		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			token := req.URL.Query().Get("token")
			target := "/admin/manage"
			if token != "" {
				target = "/admin/manage?token=" + token
			}
			http.Redirect(w, req, target, http.StatusFound)
		})

		r.Get("/manage", (&adminhandler.ManageHandler{DB: database, Tmpl: tmpl}).ServeHTTP)
		r.Get("/upload", (&adminhandler.UploadFormHandler{DB: database, Tmpl: tmpl}).ServeHTTP)
		r.Post("/upload", (&adminhandler.UploadHandler{DB: database, Tmpl: tmpl}).ServeHTTP)
		r.Get("/entry/{id}/edit", (&adminhandler.EditFormHandler{DB: database, Tmpl: tmpl}).ServeHTTP)
		r.Post("/entry/{id}/edit", (&adminhandler.EditHandler{DB: database}).ServeHTTP)
		r.Post("/entry/{id}/delete", (&adminhandler.DeleteEntryHandler{DB: database}).ServeHTTP)
		r.Post("/file/{id}/delete", (&adminhandler.DeleteFileHandler{DB: database}).ServeHTTP)
		r.Get("/analytics", (&adminhandler.AnalyticsHandler{DB: database, Tmpl: tmpl}).ServeHTTP)
		r.Get("/flags", (&adminhandler.FlagsHandler{DB: database, Tmpl: tmpl}).ServeHTTP)
		r.Get("/flags.json", (&adminhandler.FlagsJSONHandler{DB: database}).ServeHTTP)
		r.Post("/flag/{id}/resolve", (&adminhandler.ResolveFlagHandler{DB: database}).ServeHTTP)
		r.Post("/send-digest", (&adminhandler.SendDigestHandler{DB: database, EmailConfig: emailCfg}).ServeHTTP)
	})

	// Static files served from embedded FS with long-lived cache headers.
	// Cache busting is handled via the ?v= query param injected by the asset() template func.
	staticFS := http.FileServer(http.FS(embeddedFS))
	r.Handle("/static/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		staticFS.ServeHTTP(w, r)
	}))

	log.Printf("kbfirmware listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// staticVersion hashes the content of all embedded static files and returns
// the first 8 hex characters, used as a cache-busting query param.
func staticVersion(efs embed.FS) string {
	h := sha256.New()
	fs.WalkDir(efs, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := efs.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write(data)
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil))[:8]
}
