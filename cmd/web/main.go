package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/models"
	"github.com/darrenpaine/bookmarkbox/migrations"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/form/v4"
	_ "modernc.org/sqlite"
)

// The application struct holds application-wide dependencies for the web application.
type application struct {
	errorLog       *log.Logger
	infoLog        *log.Logger
	bookmarks      models.BookmarkModelInterface
	tags           models.TagModelInterface
	users          models.UserModelInterface
	templateCache  map[string]*template.Template
	router         *chi.Mux
	formDecoder    *form.Decoder
	sessionManager *scs.SessionManager
	debug          bool
}

func main() {
	// Define and parse command line options -addr and -dsn
	addr := flag.String("addr", ":4000", "HTTP network address")
	dsn := flag.String("dsn", defaultDSN, "SQLite data source name")
	verbose := flag.Bool("debug", false, "Flag to produce verbose debug messages")
	https := flag.Bool("https", false, "Flag to use TLS/https")
	maxUsers := flag.Int("max-users", 1, "Signup closes at this many users (0 for unlimited)")
	maxBookmarks := flag.Int("max-bookmarks", 0, "Maximum bookmarks per user (0 for unlimited)")
	maxTags := flag.Int("max-tags", 0, "Maximum tags per user (0 for unlimited)")
	flag.Parse()

	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	// Database connection
	db, err := openDB(*dsn)
	if err != nil {
		errorLog.Fatal(err)
	}
	defer db.Close()

	templateCache, err := newTemplateCache()
	if err != nil {
		errorLog.Fatal(err)
	}

	// Session management - 12 hour timeout
	sessionManager := scs.New()
	sessionManager.Store = sqlite3store.New(db)
	sessionManager.Lifetime = 12 * time.Hour
	sessionManager.Cookie.Secure = *https
	sessionManager.Cookie.SameSite = http.SameSiteStrictMode

	app := &application{
		errorLog:       errorLog,
		infoLog:        infoLog,
		bookmarks:      &models.BookmarkModel{DB: db, MaxBookmarks: *maxBookmarks, MaxTags: *maxTags},
		tags:           &models.TagModel{DB: db},
		users:          &models.UserModel{DB: db, MaxUsers: *maxUsers},
		templateCache:  templateCache,
		router:         chi.NewRouter(),
		formDecoder:    form.NewDecoder(),
		sessionManager: sessionManager,
		debug:          *verbose,
	}

	protection := http.NewCrossOriginProtection()
	// Can add allowed origins, for example:
	// protection.AddTrustedOrigin("https://app.example.com")
	// protection.AddTrustedOrigin("https://*.example.com")
	// protection.AddTrustedOrigin("http://localhost:3000")
	// Or bypass for public APIs or webhooks:
	// protection.AddInsecureBypassPattern("/api/webhook/")

	tlsConfig := &tls.Config{}
	srv := &http.Server{
		Addr:         *addr,
		Handler:      protection.Handler(app.router),
		ErrorLog:     errorLog,
		TLSConfig:    tlsConfig,
		IdleTimeout:  time.Minute, // prevent slowloris-style attacks
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	app.routes()
	app.infoLog.Printf("Starting server on %s", *addr)
	// TLS certificates (secret) - from Let's Encrypt
	if *https {
		err = srv.ListenAndServeTLS("./tls/cert.pem", "./tls/key.pem")
	} else {
		err = srv.ListenAndServe()
	}
	app.errorLog.Fatal(err)
}

// Local SQLite file. Foreign keys are off by default in SQLite, so turn them on
// (needed for ON DELETE CASCADE); WAL and a busy timeout let reads and writes
// from concurrent requests share the file.
const defaultDSN = "file:bookmarkbox.db?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

// The openDB() function opens the SQLite database for a given DSN and applies
// any pending schema migrations.
func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err = migrations.Up(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
