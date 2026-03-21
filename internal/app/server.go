package app

import (
	"context"
	"net"
)

func (s *Service) Start() error {
	if s.server == nil {
		return nil
	}

	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}

	return s.Serve(ln)
}

func (s *Service) Serve(ln net.Listener) error {
	if s.server == nil {
		return nil
	}

	if s.tls.Mode == "file" {
		return s.server.ServeTLS(ln, s.tls.CertFile, s.tls.KeyFile)
	}

	return s.server.Serve(ln)
}

func (s *Service) Shutdown(ctx context.Context) error {
	if s.runtimeCancel != nil {
		s.runtimeCancel()
	}

	if s.server == nil {
		return nil
	}

	return s.server.Shutdown(ctx)
}
