package server

import (
	"net"
	"net/http"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
)

type HealthServer struct {
	healthy atomic.Bool
	ready   atomic.Bool
}

func (s *HealthServer) SetHealthy(v bool) { s.healthy.Store(v) }
func (s *HealthServer) SetReady(v bool)   { s.ready.Store(v) }

func (s *HealthServer) Start(started chan<- struct{}, opts ServerOptions) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		if s.healthy.Load() {
			w.Write([]byte("OK"))
		} else {
			http.Error(w, "not healthy", http.StatusServiceUnavailable)
		}
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if s.ready.Load() {
			w.Write([]byte("OK"))
		} else {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
		}
	})

	addr := opts.GetHealthAddress()
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  opts.GetReadTimeout(),
		WriteTimeout: opts.GetWriteTimeout(),
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	if started != nil {
		started <- struct{}{}
	}

	if err := srv.Serve(l); err != nil {
		log.Fatal(err)
	}
}
