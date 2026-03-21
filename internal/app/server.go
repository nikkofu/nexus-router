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
		s.stopRuntimeWork()
		return err
	}

	return s.Serve(ln)
}

func (s *Service) Serve(ln net.Listener) error {
	if s.server == nil {
		return nil
	}

	var err error
	if s.tls.Mode == "file" {
		err = s.server.ServeTLS(ln, s.tls.CertFile, s.tls.KeyFile)
	} else {
		err = s.server.Serve(ln)
	}

	if err != nil {
		s.stopRuntimeWork()
	}

	return err
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.stopRuntimeWork()

	if s.server == nil {
		return nil
	}

	return s.server.Shutdown(ctx)
}
