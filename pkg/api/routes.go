package api

import (
	"net/http"
)

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// API routes with method-specific routing
	mux.HandleFunc("GET /api/datasources", s.HandleListDatasources)
	mux.HandleFunc("GET /api/datasources/{name}", s.HandleDatasourceBlocks)
	mux.HandleFunc("GET /api/search", s.HandleSearch)
	mux.HandleFunc("GET /api/stats", s.HandleStats)
	mux.HandleFunc("GET /health", s.HandleHealth)
}
