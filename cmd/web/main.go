package main

import (
	"crypto/tls"
	"database/sql"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"snippetbox.darrenpaine.com/internal/models"

	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/form/v4"
	_ "github.com/go-sql-driver/mysql"
)

// Define an application struct to hold the application-wide dependencies for the
// web application. For now we'll only include fields for the two custom loggers, but
// we'll add more to it as the build progresses.

type application struct {
	errorLog       *log.Logger
	infoLog        *log.Logger
	snippets       models.SnippetModelInterface
	users          models.UserModelInterface
	templateCache  map[string]*template.Template
	router         *chi.Mux
	formDecoder    *form.Decoder
	sessionManager *scs.SessionManager
}

func main() {

	// Define and parse command line options -addr and -dsn
	// Set up user and pass here to match database credentials if using automatically
	addr := flag.String("addr", ":4000", "HTTP network address")
	dsn := flag.String("dsn", "user:pass@/snippetbox?parseTime=true", "MySQL data source name")
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
	sessionManager.Store = mysqlstore.New(db)
	sessionManager.Lifetime = 12 * time.Hour
	sessionManager.Cookie.Secure = true
	sessionManager.Cookie.SameSite = http.SameSiteStrictMode

	app := &application{
		errorLog:       errorLog,
		infoLog:        infoLog,
		snippets:       &models.SnippetModel{DB: db},
		users:          &models.UserModel{DB: db},
		templateCache:  templateCache,
		router:         chi.NewRouter(),
		formDecoder:    form.NewDecoder(),
		sessionManager: sessionManager,
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
		Addr:      *addr,
		Handler:   protection.Handler(app.router),
		ErrorLog:  errorLog,
		TLSConfig: tlsConfig,
		// Add Idle, Read and Write timeouts to the server.
		IdleTimeout:  time.Minute, // prevent slowloris-style attacks
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	app.routes()
	app.infoLog.Printf("Starting server on %s", *addr)
	// Create TLS certificates to be used here
	// Otherwise, use HTTP with: err = srv.ListenAndServe()

	err = srv.ListenAndServeTLS("./tls/cert.pem", "./tls/key.pem")

	app.errorLog.Fatal(err)
}

// The openDB() function wraps sql.Open() and returns a sql.DB connection pool
// for a given DSN.
func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}
