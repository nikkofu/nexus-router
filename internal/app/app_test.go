package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServiceShutdownWaitsForBackgroundRuntimeToStop(t *testing.T) {
	done := make(chan struct{})
	cancelCalled := make(chan struct{})

	svc := &Service{
		runtimeCancel: func() {
			close(cancelCalled)
		},
		runtimeDone: done,
	}

	result := make(chan error, 1)
	go func() {
		result <- svc.Shutdown(context.Background())
	}()

	select {
	case <-cancelCalled:
	case <-time.After(time.Second):
		t.Fatal("Shutdown() did not cancel runtime work")
	}

	select {
	case err := <-result:
		t.Fatalf("Shutdown() returned before background runtime stopped: %v", err)
	default:
	}

	close(done)

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Shutdown() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Shutdown() did not return after background runtime stopped")
	}
}

func TestServiceStartFailureCleansUpBackgroundRuntime(t *testing.T) {
	done := make(chan struct{})
	cancelCalled := make(chan struct{})
	releaseDone := make(chan struct{})

	svc := &Service{
		server: &http.Server{
			Addr: "bad-addr",
		},
		runtimeCancel: func() {
			close(cancelCalled)
			go func() {
				<-releaseDone
				close(done)
			}()
		},
		runtimeDone: done,
	}

	result := make(chan error, 1)
	go func() {
		result <- svc.Start()
	}()

	select {
	case <-cancelCalled:
	case <-time.After(time.Second):
		t.Fatal("Start() did not cancel runtime work after listen failure")
	}

	select {
	case err := <-result:
		t.Fatalf("Start() returned before background runtime stopped: %v", err)
	default:
	}

	close(releaseDone)

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Start() error = nil, want listen failure")
		}
	case <-time.After(time.Second):
		t.Fatal("Start() did not return after background runtime stopped")
	}
}

func TestServiceStartAdminListenFailureCleansUpBackgroundRuntime(t *testing.T) {
	done := make(chan struct{})
	cancelCalled := make(chan struct{})
	releaseDone := make(chan struct{})

	svc := &Service{
		server: &http.Server{
			Addr: "127.0.0.1:0",
		},
		adminServer: &http.Server{
			Addr: "bad-addr",
		},
		runtimeCancel: func() {
			close(cancelCalled)
			go func() {
				<-releaseDone
				close(done)
			}()
		},
		runtimeDone: done,
	}

	result := make(chan error, 1)
	go func() {
		result <- svc.Start()
	}()

	select {
	case <-cancelCalled:
	case <-time.After(time.Second):
		t.Fatal("Start() did not cancel runtime work after admin listen failure")
	}

	select {
	case err := <-result:
		t.Fatalf("Start() returned before background runtime stopped: %v", err)
	default:
	}

	close(releaseDone)

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Start() error = nil, want admin listen failure")
		}
	case <-time.After(time.Second):
		t.Fatal("Start() did not return after background runtime stopped")
	}
}

func TestServiceServeErrorCleansUpBackgroundRuntime(t *testing.T) {
	done := make(chan struct{})
	cancelCalled := make(chan struct{})
	releaseDone := make(chan struct{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	svc := &Service{
		server: &http.Server{
			Handler: http.NotFoundHandler(),
		},
		runtimeCancel: func() {
			close(cancelCalled)
			go func() {
				<-releaseDone
				close(done)
			}()
		},
		runtimeDone: done,
	}

	result := make(chan error, 1)
	go func() {
		result <- svc.Serve(ln)
	}()

	select {
	case <-cancelCalled:
	case <-time.After(time.Second):
		t.Fatal("Serve() did not cancel runtime work after serve error")
	}

	select {
	case err := <-result:
		t.Fatalf("Serve() returned before background runtime stopped: %v", err)
	default:
	}

	close(releaseDone)

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Serve() error = nil, want listener failure")
		}
		if errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Serve() error = %v, want non-shutdown serve failure", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not return after background runtime stopped")
	}
}
