package server

import (
	"encoding/json"
	"net/http"
)

type Config struct {
	Version string
}

type statusResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

func New(cfg Config) http.Handler {
	version := cfg.Version
	if version == "" {
		version = "dev"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		response := statusResponse{
			Status:  "ok",
			Service: "controller",
			Version: version,
		}

		body, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		_, _ = w.Write([]byte("\n"))
	})

	return mux
}
