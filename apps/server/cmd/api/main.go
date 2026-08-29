package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"yujian.me/server/internal/assets"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/config"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/contract"
	"yujian.me/server/internal/httpapi"
	"yujian.me/server/internal/ports"
	"yujian.me/server/internal/providers/local"
	"yujian.me/server/internal/publish"
	"yujian.me/server/internal/store/memory"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	settings, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, settings, logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, settings config.Config, logger *slog.Logger) error {
	handler, err := buildHandler(settings, ServiceDependencies{})
	if err != nil {
		return err
	}
	return runServer(ctx, settings, logger, handler)
}

type ServiceDependencies struct {
	Content httpapi.ContentService
	Assets  httpapi.AssetService
	Publish httpapi.PublishService
}

func buildHandler(settings config.Config, dependencies ServiceDependencies) (http.Handler, error) {
	if settings.Environment != "production" && dependencies.Content == nil && dependencies.Assets == nil && dependencies.Publish == nil {
		dependencies = developmentDependencies()
	}
	if settings.Environment == "production" && (dependencies.Content == nil || dependencies.Assets == nil || dependencies.Publish == nil) {
		return nil, errors.New("production services are not fully configured")
	}
	var identityProvider ports.IdentityProvider
	if settings.OIDCIssuer != "" && settings.OIDCAudience != "" {
		provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
			Issuer:       settings.OIDCIssuer,
			Audience:     settings.OIDCAudience,
			RequireHTTPS: settings.Environment == "production",
		})
		if err != nil {
			return nil, err
		}
		identityProvider = provider
	}
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{
		Environment:      settings.Environment,
		AllowDevIdentity: settings.AllowDevIdentity,
		IdentityProvider: identityProvider,
	})
	if err != nil {
		return nil, err
	}
	return httpapi.NewRouter(httpapi.RouterOptions{
		Content:    dependencies.Content,
		Assets:     dependencies.Assets,
		Publish:    dependencies.Publish,
		Middleware: middleware,
	}), nil
}

func developmentDependencies() ServiceDependencies {
	state := memory.NewState()
	validator := contract.NewValidator()
	blobs := local.NewBlobStore()
	trigger := local.NewBuildTrigger()
	return ServiceDependencies{
		Content: content.NewService(content.ServiceOptions{
			Store:     memory.NewContentRepository(state),
			Validator: validator,
		}),
		Assets: assets.NewService(assets.ServiceOptions{
			Repository: memory.NewAssetRepository(state),
			BlobStore:  blobs,
		}),
		Publish: publish.NewService(publish.ServiceOptions{
			Repository:   memory.NewPublishRepository(state),
			BlobStore:    blobs,
			BuildTrigger: trigger,
			Validator:    validator,
		}),
	}
}

func runServer(ctx context.Context, settings config.Config, logger *slog.Logger, handler http.Handler) error {
	handler = httpapi.LoggingMiddleware(handler, logger)

	server := &http.Server{
		Addr:              settings.Address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErrors := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "address", settings.Address)
		serveErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), settings.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return err
		}
		return nil
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func healthHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
}
