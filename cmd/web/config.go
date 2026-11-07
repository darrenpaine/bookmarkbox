package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
)

// config holds the settings read at start-up. Environment variables set the
// defaults, and command-line flags override them.
type config struct {
	addr         string
	dbPath       string
	cookieSecure bool
	debug        bool
	maxUsers     int
	maxBookmarks int
	maxTags      int
}

// parseConfig reads the settings. An invalid value is an error rather than
// being ignored, so a typo can't quietly leave a limit switched off.
func parseConfig(args []string, getenv func(string) string) (config, error) {
	port, err := envPort(getenv)
	if err != nil {
		return config{}, err
	}
	dbPath := getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./bookmarkbox.db"
	}
	cookieSecure, err := envBool(getenv, "COOKIE_SECURE", true)
	if err != nil {
		return config{}, err
	}
	maxUsers, err := envLimit(getenv, "MAX_USERS", 1)
	if err != nil {
		return config{}, err
	}
	maxBookmarks, err := envLimit(getenv, "MAX_BOOKMARKS_PER_USER", 0)
	if err != nil {
		return config{}, err
	}
	maxTags, err := envLimit(getenv, "MAX_TAGS_PER_USER", 0)
	if err != nil {
		return config{}, err
	}

	var cfg config
	fs := flag.NewFlagSet("bookmarkbox", flag.ContinueOnError)
	fs.StringVar(&cfg.addr, "addr", ":"+port, "HTTP network address")
	fs.StringVar(&cfg.dbPath, "db", dbPath, "SQLite database file")
	fs.BoolVar(&cfg.cookieSecure, "cookie-secure", cookieSecure, "Send the session cookie over HTTPS only")
	fs.BoolVar(&cfg.debug, "debug", false, "Show stack traces in the browser (local use only)")
	fs.IntVar(&cfg.maxUsers, "max-users", maxUsers, "Signup closes at this many users (0 for unlimited)")
	fs.IntVar(&cfg.maxBookmarks, "max-bookmarks", maxBookmarks, "Maximum bookmarks per user (0 for unlimited)")
	fs.IntVar(&cfg.maxTags, "max-tags", maxTags, "Maximum tags per user (0 for unlimited)")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	for _, limit := range []struct {
		flag  string
		value int
	}{
		{"-max-users", cfg.maxUsers},
		{"-max-bookmarks", cfg.maxBookmarks},
		{"-max-tags", cfg.maxTags},
	} {
		if limit.value < 0 {
			return config{}, fmt.Errorf("%s must be 0 or more", limit.flag)
		}
	}
	return cfg, nil
}

func envPort(getenv func(string) string) (string, error) {
	value := getenv("PORT")
	if value == "" {
		return "4000", nil
	}
	if port, err := strconv.Atoi(value); err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("PORT: %q is not a port number", value)
	}
	return value, nil
}

func envBool(getenv func(string) string, name string, fallback bool) (bool, error) {
	value := getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s: %q is not true or false", name, value)
	}
	return parsed, nil
}

func envLimit(getenv func(string) string, name string, fallback int) (int, error) {
	value := getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s: %q is not 0 or a positive whole number", name, value)
	}
	return parsed, nil
}

// sqliteDSN builds the connection string for a database file. Foreign keys are
// off by default in SQLite, so turn them on (needed for ON DELETE CASCADE); WAL
// and a busy timeout let reads and writes from concurrent requests share the
// file. The characters that end a path in a URI are escaped.
func sqliteDSN(path string) string {
	return "file:" + strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(path) +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
}
