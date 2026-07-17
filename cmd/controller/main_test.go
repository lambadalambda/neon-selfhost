package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestShutdownControllerDrainsHandlersBeforeClosingResources(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	resourceClosed := make(chan struct{})
	server, address := startTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		close(finished)
		w.WriteHeader(http.StatusNoContent)
	}))

	requestDone := make(chan error, 1)
	go func() {
		res, err := http.Get(address)
		if err == nil {
			_ = res.Body.Close()
		}
		requestDone <- err
	}()
	<-started

	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		shutdownDone <- shutdownController(ctx, server, namedCloser{
			name: "test resource",
			closer: closeFunc(func() error {
				select {
				case <-finished:
				default:
					t.Error("resource closed before in-flight handler finished")
				}
				close(resourceClosed)
				return nil
			}),
		})
	}()

	select {
	case <-resourceClosed:
		t.Fatal("resource closed while handler was blocked")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-shutdownDone; err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if err := <-requestDone; err != nil {
		t.Fatalf("request: %v", err)
	}
	<-resourceClosed
}

func TestShutdownControllerClosesResourcesAfterTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	resourceClosed := make(chan struct{})
	server, address := startTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		res, err := http.Get(address)
		if err == nil {
			_ = res.Body.Close()
		}
	}()
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := shutdownController(ctx, server, namedCloser{
		name: "test resource",
		closer: closeFunc(func() error {
			close(resourceClosed)
			return nil
		}),
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected shutdown timeout, got %v", err)
	}
	<-resourceClosed
	close(release)
	_ = server.Close()
	<-requestDone
}

func TestShutdownControllerContinuesAfterResourceCloseFailure(t *testing.T) {
	server := &http.Server{}
	closeErr := errors.New("cannot close first resource")
	secondClosed := false
	err := shutdownController(context.Background(), server,
		namedCloser{name: "first", closer: closeFunc(func() error { return closeErr })},
		namedCloser{name: "second", closer: closeFunc(func() error {
			secondClosed = true
			return nil
		})},
	)
	if !errors.Is(err, closeErr) {
		t.Fatalf("expected close error, got %v", err)
	}
	if !secondClosed {
		t.Fatal("expected remaining resources to close after an earlier failure")
	}
}

func startTestServer(t *testing.T, handler http.Handler) (*http.Server, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	return server, "http://" + listener.Addr().String()
}

type closeFunc func() error

func (f closeFunc) Close() error {
	return f()
}

var _ io.Closer = closeFunc(nil)
