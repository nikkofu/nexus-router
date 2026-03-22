package app

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/nikkofu/nexus-router/internal/config"
)

func (s *Service) Start() error {
	if s.server == nil {
		return nil
	}

	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		s.stopRuntimeWork()
		return err
	}

	if s.adminServer != nil {
		adminLn, err := net.Listen("tcp", s.adminServer.Addr)
		if err != nil {
			_ = ln.Close()
			s.stopRuntimeWork()
			return err
		}
		return s.serveAll(ln, adminLn)
	}

	return s.Serve(ln)
}

func (s *Service) Serve(ln net.Listener) error {
	if s.server == nil {
		return nil
	}

	err := serveHTTP(s.server, ln, s.tls)
	if err != nil {
		s.stopRuntimeWork()
	}

	return err
}

func (s *Service) serveAll(publicLn net.Listener, adminLn net.Listener) error {
	resultCh := make(chan error, 2)

	go func() {
		resultCh <- serveHTTP(s.server, publicLn, s.tls)
	}()
	go func() {
		resultCh <- serveHTTP(s.adminServer, adminLn, s.tls)
	}()

	firstErr := <-resultCh
	if firstErr != nil && !errors.Is(firstErr, http.ErrServerClosed) {
		if s.adminServer != nil {
			_ = s.adminServer.Close()
		}
		if s.server != nil {
			_ = s.server.Close()
		}
	}

	secondErr := <-resultCh
	err := selectServeError(firstErr, secondErr)
	if err != nil {
		s.stopRuntimeWork()
	}

	return err
}

func serveHTTP(server *http.Server, ln net.Listener, tls config.TLSConfig) error {
	if server == nil {
		return nil
	}

	var err error
	if tls.Mode == "file" {
		err = server.ServeTLS(ln, tls.CertFile, tls.KeyFile)
	} else {
		err = server.Serve(ln)
	}

	return err
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.stopRuntimeWork()

	var firstErr error
	if s.adminServer != nil {
		if err := s.adminServer.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func selectServeError(errs ...error) error {
	for _, err := range errs {
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			continue
		}
		return err
	}

	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}
