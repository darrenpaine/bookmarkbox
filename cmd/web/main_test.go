package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

// A shutdown must wait for a request already in progress.
func TestServeFinishesRequestsOnShutdown(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Write([]byte("finished"))
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NilError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveResult := make(chan error, 1)
	go func() { serveResult <- serve(ctx, srv, listener, slog.New(slog.DiscardHandler)) }()

	type response struct {
		body string
		err  error
	}
	responses := make(chan response, 1)
	go func() {
		rs, err := http.Get("http://" + listener.Addr().String())
		if err != nil {
			responses <- response{err: err}
			return
		}
		defer rs.Body.Close()
		body, err := io.ReadAll(rs.Body)
		responses <- response{string(body), err}
	}()

	<-started
	cancel()
	select {
	case err := <-serveResult:
		t.Fatalf("serve returned before the request finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	got := <-responses
	assert.NilError(t, got.err)
	assert.Equal(t, got.body, "finished")
	assert.NilError(t, <-serveResult)
}

// A server that can't run is reported, not treated as a clean shutdown.
func TestServeReturnsServerErrors(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NilError(t, err)
	listener.Close()

	err = serve(context.Background(), &http.Server{}, listener, slog.New(slog.DiscardHandler))
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		t.Errorf("got %v, want a listener error", err)
	}
}
