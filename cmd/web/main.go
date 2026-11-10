package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
	logger         *slog.Logger
	bookmarks      models.BookmarkModelInterface
	tags           models.TagModelInterface
	users          models.UserModelInterface
	imports        models.ImportModelInterface
	templateCache  map[string]*template.Template
	router         *chi.Mux
	formDecoder    *form.Decoder
	sessionManager *scs.SessionManager
	debug          bool
}

// How long requests get to finish on shutdown. Docker kills a container 10
// seconds after asking it to stop, so this leaves time to close the database.
const shutdownTimeout = 8 * time.Second

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := parseConfig(os.Args[1:], os.Getenv)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		logger.Error(err.Error())
		os.Exit(2)
	}
	if err := run(cfg, logger); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}

// run serves the application until it gets an interrupt or terminate signal.
func run(cfg config, logger *slog.Logger) error {
	// Database connection
	db, err := openDB(sqliteDSN(cfg.dbPath))
	if err != nil {
		return err
	}
	defer db.Close()

	templateCache, err := newTemplateCache()
	if err != nil {
		return err
	}

	// Session management - 12 hour timeout
	sessionStore := sqlite3store.New(db)
	defer sessionStore.StopCleanup()
	sessionManager := scs.New()
	sessionManager.Store = sessionStore
	sessionManager.Lifetime = 12 * time.Hour
	sessionManager.Cookie.Secure = cfg.cookieSecure
	sessionManager.Cookie.SameSite = http.SameSiteStrictMode

	app := &application{
		logger:         logger,
		bookmarks:      &models.BookmarkModel{DB: db, MaxBookmarks: cfg.maxBookmarks, MaxTags: cfg.maxTags},
		imports:        &models.ImportModel{DB: db},
		tags:           &models.TagModel{DB: db},
		users:          &models.UserModel{DB: db, MaxUsers: cfg.maxUsers},
		templateCache:  templateCache,
		router:         chi.NewRouter(),
		formDecoder:    form.NewDecoder(),
		sessionManager: sessionManager,
		debug:          cfg.debug,
	}

	protection := http.NewCrossOriginProtection()
	// Can add allowed origins, for example:
	// protection.AddTrustedOrigin("https://app.example.com")
	// protection.AddTrustedOrigin("https://*.example.com")
	// protection.AddTrustedOrigin("http://localhost:3000")
	// Or bypass for public APIs or webhooks:
	// protection.AddInsecureBypassPattern("/api/webhook/")

	srv := &http.Server{
		Handler:      protection.Handler(app.routes()),
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
		IdleTimeout:  time.Minute, // prevent slowloris-style attacks
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	listener, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Info("starting server", "addr", cfg.addr)
	return serve(ctx, srv, listener, logger)
}

// serve runs srv until ctx is cancelled, then lets requests in progress finish.
func serve(ctx context.Context, srv *http.Server, listener net.Listener, logger *slog.Logger) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(listener) }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		srv.Close()
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("stopped")
	return nil
}

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
