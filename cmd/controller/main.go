package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"neon-selfhost/internal/branch"
	"neon-selfhost/internal/config"
	"neon-selfhost/internal/preflight"
	"neon-selfhost/internal/server"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	if err := preflight.CheckControllerDataDir(cfg.ControllerDataDir); err != nil {
		logger.Error("startup preflight", "error", err)
		os.Exit(1)
	}
	if err := preflight.CheckControllerDataDir(cfg.ComputeDataDir); err != nil {
		logger.Error("compute data preflight", "error", err)
		os.Exit(1)
	}

	branchStore := branch.NewStore()
	branchStoreMode := "memory"
	branchSchemaVersion := 0
	if cfg.ControllerDataDir != "" {
		sqliteStore, err := branch.NewSQLitePersistentStore(cfg.ControllerDataDir)
		if err != nil {
			logger.Error("init sqlite branch store", "error", err)
			os.Exit(1)
		}
		branchStore = sqliteStore
		branchStoreMode = "sqlite"
		branchSchemaVersion = branch.SQLiteBranchSchemaVersion
	}

	endpointSelectionPath := ""
	branchDBPath := ""
	operationDBPath := ""
	legacyOperationLogPath := ""
	if cfg.ComputeDataDir != "" {
		endpointSelectionPath = filepath.Join(cfg.ComputeDataDir, "endpoint-selection.json")
	}
	if cfg.ControllerDataDir != "" {
		branchDBPath = filepath.Join(cfg.ControllerDataDir, "controller.db")
		operationDBPath = filepath.Join(cfg.ControllerDataDir, "operations.db")
		legacyOperationLogPath = filepath.Join(cfg.ControllerDataDir, "operations.jsonl")
	}

	primaryEndpoint := server.PrimaryEndpointController(server.NewInMemoryPrimaryEndpointController(
		cfg.PrimaryEndpointHost,
		cfg.PrimaryEndpointPort,
		cfg.PrimaryEndpointDatabase,
		cfg.PrimaryEndpointUser,
		cfg.PrimaryEndpointPassword,
		endpointSelectionPath,
	))

	branchAttachmentResolver := server.NewNoopBranchAttachmentResolver()
	branchEndpoints := server.NewNoopBranchEndpointController(
		cfg.PrimaryEndpointHost,
		cfg.PrimaryEndpointDatabase,
		cfg.PrimaryEndpointUser,
	)

	if cfg.PrimaryEndpointMode == "docker" {
		dockerPrimaryEndpoint, err := server.NewDockerPrimaryEndpointController(server.DockerPrimaryEndpointOptions{
			SocketPath:     cfg.DockerSocketPath,
			ComposeProject: cfg.DockerComposeProject,
			Service:        cfg.PrimaryEndpointService,
			Host:           cfg.PrimaryEndpointHost,
			Port:           cfg.PrimaryEndpointPort,
			Database:       cfg.PrimaryEndpointDatabase,
			User:           cfg.PrimaryEndpointUser,
			Password:       cfg.PrimaryEndpointPassword,
			SelectionPath:  endpointSelectionPath,
		})
		if err != nil {
			logger.Error("init docker primary endpoint controller", "error", err)
			os.Exit(1)
		}

		primaryEndpoint = dockerPrimaryEndpoint

		pageserverResolver, err := server.NewPageserverBranchAttachmentResolver(server.PageserverBranchAttachmentOptions{
			Store:     branchStore,
			BaseURL:   cfg.PageserverAPI,
			PGVersion: cfg.PageserverPGVersion,
		})
		if err != nil {
			logger.Error("init pageserver branch attachment resolver", "error", err)
			os.Exit(1)
		}

		branchAttachmentResolver = pageserverResolver

		dockerBranchEndpoints, err := server.NewDockerBranchEndpointController(server.DockerBranchEndpointOptions{
			Store:          branchStore,
			SocketPath:     cfg.DockerSocketPath,
			ComposeProject: cfg.DockerComposeProject,
			AdvertisedHost: cfg.PrimaryEndpointHost,
			BindHost:       cfg.BranchEndpointBindHost,
			PortStart:      cfg.BranchEndpointPortStart,
			PortEnd:        cfg.BranchEndpointPortEnd,
			Database:       cfg.PrimaryEndpointDatabase,
			User:           cfg.PrimaryEndpointUser,
			ComputeDataDir: cfg.ComputeDataDir,
			PGVersion:      cfg.PageserverPGVersion,
			IdleTimeout:    cfg.BranchEndpointIdleStop,
			MaxActiveConns: cfg.BranchEndpointMaxConns,
			Logger:         logger.With("component", "branch_endpoints"),
		})
		if err != nil {
			logger.Error("init docker branch endpoint controller", "error", err)
			os.Exit(1)
		}

		branchEndpoints = dockerBranchEndpoints
	}

	handler := server.New(server.Config{
		Version:                  version,
		BranchStore:              branchStore,
		BranchAttachmentResolver: branchAttachmentResolver,
		PrimaryEndpoint:          primaryEndpoint,
		BranchEndpoints:          branchEndpoints,
		BasicAuthUser:            cfg.BasicAuthUser,
		BasicAuthPassword:        cfg.BasicAuthPassword,
		OperationDBPath:          operationDBPath,
		LegacyOperationLogPath:   legacyOperationLogPath,
		BranchStoreMode:          branchStoreMode,
		BranchDBPath:             branchDBPath,
		BranchSchemaVersion:      branchSchemaVersion,
		Logger:                   logger.With("component", "http_api"),
	})
	var handlerCloser interface{ Close() error }
	if closer, ok := handler.(interface{ Close() error }); ok {
		handlerCloser = closer
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	listener, err := net.Listen("tcp", cfg.Addr())
	if err != nil {
		logger.Error("listen", "addr", cfg.Addr(), "error", err)
		os.Exit(1)
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("controller listening", "addr", listener.Addr().String(), "version", version)
		if serveErr := httpServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case serveErr := <-errCh:
		logger.Error("serve http", "error", serveErr)
		os.Exit(1)
	case sig := <-sigCh:
		logger.Info("received signal, shutting down", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := branchStore.Close(); err != nil {
		logger.Error("shutdown branch store", "error", err)
	}

	if err := branchEndpoints.Close(); err != nil {
		logger.Error("shutdown branch endpoints", "error", err)
	}

	if handlerCloser != nil {
		if err := handlerCloser.Close(); err != nil {
			logger.Error("shutdown handler resources", "error", err)
		}
	}

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("shutdown http server", "error", err)
		os.Exit(1)
	}

	logger.Info("controller shutdown complete")
}
