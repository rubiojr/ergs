package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/rubiojr/ergs/cmd/web/components/types"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/shared"
	"github.com/rubiojr/ergs/pkg/storage"
)

type Server struct {
	registry       *core.Registry
	storageManager *storage.Manager
}

func NewServer(registry *core.Registry, storageManager *storage.Manager) *Server {
	return &Server{
		registry:       registry,
		storageManager: storageManager,
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, error, message string) {
	response := ErrorResponse{
		Error:   error,
		Message: message,
	}
	s.writeJSON(w, status, response)
}

func (s *Server) getDatasourceList() []types.DatasourceInfo {
	return shared.GetDatasourceList(s.registry, s.storageManager)
}

func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
