package utils

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

type KeepAliveService struct {
	*http.Server
	lis      net.Listener
	tlsConf  *tls.Config
	endpoint *url.URL
	router   *http.ServeMux
}

func NewKeepAliveService(tlsConf *tls.Config) *KeepAliveService {
	srv := &KeepAliveService{
		tlsConf: tlsConf,
	}
	srv.Server = &http.Server{
		TLSConfig: srv.tlsConf,
	}

	srv.router = http.NewServeMux()
	srv.Server.Handler = srv.router

	return srv
}

func (s *KeepAliveService) Start() error {
	if err := s.generateEndpoint(); err != nil {
		return err
	}

	s.router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"method":"health"}`)
	})

	log.Debugf("keepalive service started at %s", s.endpoint)

	var err error
	if s.tlsConf != nil {
		err = s.ServeTLS(s.lis, "", "")
	} else {
		err = s.Serve(s.lis)
	}
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func (s *KeepAliveService) Stop(ctx context.Context) error {
	return s.Shutdown(ctx)
}

func (s *KeepAliveService) generatePort(min, max int) int {
	return rand.Intn(max-min) + min
}

func (s *KeepAliveService) generateEndpoint() error {
	if s.endpoint != nil {
		return nil
	}

	for {
		port := s.generatePort(10000, 65535)
		addr := fmt.Sprintf(":%d", port)
		lis, err := net.Listen("tcp", addr)
		if err == nil && lis != nil {
			s.lis = lis
			endpoint, _ := url.Parse("tcp://" + addr)
			s.endpoint = endpoint
			return nil
		}
	}
}

func (s *KeepAliveService) Endpoint() (*url.URL, error) {
	if err := s.generateEndpoint(); err != nil {
		return nil, err
	}
	return s.endpoint, nil
}
