package app

import "context"

func (s *Service) Start() error {
	if s.server == nil {
		return nil
	}

	return s.server.ListenAndServe()
}

func (s *Service) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	return s.server.Shutdown(ctx)
}
