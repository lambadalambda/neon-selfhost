package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"neon-selfhost/internal/branch"
	"neon-selfhost/internal/config"
	"neon-selfhost/internal/preflight"
	"neon-selfhost/internal/server"
)

var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := preflight.CheckControllerDataDir(cfg.ControllerDataDir); err != nil {
		log.Fatalf("startup preflight: %v", err)
	}

	primaryEndpoint := server.PrimaryEndpointController(server.NewInMemoryPrimaryEndpointController(
		cfg.PrimaryEndpointHost,
		cfg.PrimaryEndpointPort,
		cfg.PrimaryEndpointDatabase,
		cfg.PrimaryEndpointUser,
	))

	if cfg.PrimaryEndpointMode == "docker" {
		dockerPrimaryEndpoint, err := server.NewDockerPrimaryEndpointController(server.DockerPrimaryEndpointOptions{
			SocketPath:     cfg.DockerSocketPath,
			ComposeProject: cfg.DockerComposeProject,
			Service:        cfg.PrimaryEndpointService,
			Host:           cfg.PrimaryEndpointHost,
			Port:           cfg.PrimaryEndpointPort,
			Database:       cfg.PrimaryEndpointDatabase,
			User:           cfg.PrimaryEndpointUser,
		})
		if err != nil {
			log.Fatalf("init docker primary endpoint controller: %v", err)
		}

		primaryEndpoint = dockerPrimaryEndpoint
	}

	branchStore := branch.NewStore()
	if cfg.ControllerDataDir != "" {
		persistentStore, err := branch.NewPersistentStore(cfg.ControllerDataDir)
		if err != nil {
			log.Fatalf("init persistent branch store: %v", err)
		}
		branchStore = persistentStore
	}

	handler := server.New(server.Config{
		Version:           version,
		BranchStore:       branchStore,
		PrimaryEndpoint:   primaryEndpoint,
		BasicAuthUser:     cfg.BasicAuthUser,
		BasicAuthPassword: cfg.BasicAuthPassword,
	})
	httpServer := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	listener, err := net.Listen("tcp", cfg.Addr())
	if err != nil {
		log.Fatalf("listen on %s: %v", cfg.Addr(), err)
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("controller listening on %s (version=%s)", listener.Addr().String(), version)
		if serveErr := httpServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case serveErr := <-errCh:
		log.Fatalf("serve http: %v", serveErr)
	case sig := <-sigCh:
		log.Printf("received signal %s, shutting down", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown http server: %v", err)
	}

	log.Print("controller shutdown complete")
}
