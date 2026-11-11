package main

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Login, signup and password change each allow this many attempts a minute
// from one client, as a burst or spread out. bcrypt is slow on purpose, so a
// flood of attempts could otherwise stall a small server, and guessing
// passwords gets slow.
const (
	attemptsPerMinute = 10
	attemptsBurst     = 10
	attemptsMaxKeys   = 10_000 // clients tracked at once; past this, new ones wait
)

const tooManyAttemptsMessage = "Too many attempts. Try again in a minute."

// attemptLimiter counts attempts per key. In memory today; a shared store can
// take its place when there's more than one server.
type attemptLimiter interface {
	Allow(key string) bool
}

// clientIP is the address of the client making the request. Behind a proxy
// the connection comes from the proxy, so CLIENT_IP_HEADER names the header
// it puts the client's address in. Without one, a header can't be trusted:
// anyone can send it.
func (app *application) clientIP(r *http.Request) string {
	if app.clientIPHeader != "" {
		if addr, ok := headerIP(r.Header, app.clientIPHeader); ok {
			return addr.String()
		}
		if app.clientIPHeaderWarned.CompareAndSwap(false, true) {
			app.logger.Warn("client IP header missing or invalid; using the connection's address",
				"header", app.clientIPHeader, "remote", r.RemoteAddr)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// headerIP reads an address from the named header. X-Forwarded-For lists
// every hop, and only the last one was added by our own proxy: the ones
// before it came from the client and could be anything.
func headerIP(header http.Header, name string) (netip.Addr, bool) {
	values := header.Values(name)
	if len(values) == 0 {
		return netip.Addr{}, false
	}
	value := values[len(values)-1]
	if name == "X-Forwarded-For" {
		value = value[strings.LastIndexByte(value, ',')+1:]
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// allowAttempt reports whether the client may try this route again now. An
// IPv6 client is counted by its /64, since one home or server is usually
// given a whole /64 and could otherwise use a new address for every attempt.
func (app *application) allowAttempt(r *http.Request) bool {
	client := app.clientIP(r)
	if addr, err := netip.ParseAddr(client); err == nil && addr.Is6() {
		client = netip.PrefixFrom(addr, 64).Masked().String()
	}
	return app.attempts.Allow(r.URL.Path + " " + client)
}

// tooManyAttempts shows the form again with the rate limit message, keeping
// what was typed apart from passwords.
func (app *application) tooManyAttempts(w http.ResponseWriter, r *http.Request, page string, form any) {
	app.logger.Warn("rate limited", "ip", app.clientIP(r), "uri", r.URL.RequestURI())
	data := app.newTemplateData(r)
	data.Form = form
	w.Header().Set("Retry-After", "60")
	app.render(w, http.StatusTooManyRequests, page, data)
}
