package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"yujian.me/server/internal/adminweb"
	"yujian.me/server/internal/assets"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/config"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/contract"
	"yujian.me/server/internal/httpapi"
	"yujian.me/server/internal/ports"
	"yujian.me/server/internal/providers/local"
	"yujian.me/server/internal/providers/pages"
	providerS3 "yujian.me/server/internal/providers/s3"
	"yujian.me/server/internal/publish"
	"yujian.me/server/internal/publisher"
	"yujian.me/server/internal/sitegen"
	"yujian.me/server/internal/store/memory"
	"yujian.me/server/internal/store/postgres"
	"yujian.me/server/internal/webassets"
)

// main validates startup settings and translates process signals into the
// cancellation used by HTTP, reconciliation and dependency cleanup.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if handled, err := runCommand(os.Args[1:], os.Stdout); handled {
		if err != nil {
			logger.Error("export site", "error", err)
			os.Exit(1)
		}
		return
	}
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

// run selects environment-specific services and shuts down the HTTP server and
// reconciler before releasing storage, preserving cleanup errors on exit.
func run(ctx context.Context, settings config.Config, logger *slog.Logger) (returnErr error) {
	dependencies := ServiceDependencies{}
	closeResources := func() error { return nil }
	if settings.Environment == "production" {
		var err error
		dependencies, closeResources, err = buildProductionDependencies(ctx, settings, defaultProductionFactory())
		if err != nil {
			return err
		}
	} else {
		dependencies = developmentDependencies()
		closeResources = dependencies.Close
	}
	defer func() { returnErr = errors.Join(returnErr, closeResources()) }()
	handler, err := buildHandler(settings, dependencies)
	if err != nil {
		return err
	}
	if dependencies.PublishReconciler != nil || dependencies.PublisherReconciler != nil {
		stopReconciler := startPublishReconciler(ctx, 15*time.Second, serviceReconcilers{worker: dependencies.PublisherReconciler, publish: dependencies.PublishReconciler}, logger)
		defer stopReconciler()
	}
	return runServer(ctx, settings, logger, handler)
}

// startPublishReconciler starts one cancellable reconciliation loop and returns
// an idempotent stop function that waits for the active reconciliation to exit.
func startPublishReconciler(
	parent context.Context,
	interval time.Duration,
	reconciler publishReconciler,
	logger *slog.Logger,
) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runPublishReconciler(ctx, interval, reconciler, logger)
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

type publishReconciler interface {
	Reconcile(context.Context) error
}

// serviceReconcilers advances durable builds before updating published pointers.
type serviceReconcilers struct{ worker, publish publishReconciler }

// Reconcile runs both boundaries in order while retaining independent failures.
func (services serviceReconcilers) Reconcile(ctx context.Context) error {
	var result error
	if services.worker != nil {
		result = services.worker.Reconcile(ctx)
	}
	if services.publish != nil {
		result = errors.Join(result, services.publish.Reconcile(ctx))
	}
	return result
}

type ServiceDependencies struct {
	Content             httpapi.ContentService
	Assets              httpapi.AssetService
	Publish             httpapi.PublishService
	PublishReconciler   publishReconciler
	PublisherReconciler publishReconciler
	Publisher           *publisher.Service
	AdminFiles          fs.FS
	LocalUploads        http.Handler
	Close               func() error
	Sessions            auth.SessionStore
}

type productionDatabase interface {
	postgres.Executor
	postgres.PublisherLocker
	Close() error
}

type productionFactory struct {
	openDatabase func(context.Context, string) (productionDatabase, error)
	newBlobStore func(providerS3.Config) (ports.BlobStore, error)
	newPages     func(pages.Config) (publisher.Pages, error)
	newGenerator func() (siteGenerator, error)
	newAdmin     func() (fs.FS, error)
	readSnapshot func(context.Context, ports.BlobStore, string) (io.ReadCloser, error)
}

// siteGenerator freezes compiled renderer identity alongside its build operation.
type siteGenerator interface {
	Generate(context.Context, []byte) (map[string][]byte, error)
	Identity() string
}

// defaultProductionFactory uses in-process rendering and direct provider clients.
func defaultProductionFactory() productionFactory {
	return productionFactory{
		openDatabase: func(ctx context.Context, databaseURL string) (productionDatabase, error) {
			return postgres.Open(ctx, databaseURL)
		},
		newBlobStore: func(settings providerS3.Config) (ports.BlobStore, error) {
			return providerS3.NewBlobStore(settings)
		},
		newPages: func(settings pages.Config) (publisher.Pages, error) {
			return pages.NewClient(settings)
		},
		newGenerator: embeddedGenerator,
		newAdmin:     webassets.Admin,
		readSnapshot: readStoredSnapshot,
	}
}

// embeddedGenerator refuses missing renderer or browser resources at startup.
func embeddedGenerator() (siteGenerator, error) {
	renderer, err := webassets.Renderer()
	if err != nil {
		return nil, err
	}
	files, err := webassets.Public()
	if err != nil {
		return nil, err
	}
	generator, err := sitegen.New(renderer, files)
	if err != nil {
		return nil, err
	}
	return freezeExecutableIdentity(generator)
}

// readStoredSnapshot uses authenticated object reads without browser signed URLs.
func readStoredSnapshot(ctx context.Context, blobs ports.BlobStore, key string) (io.ReadCloser, error) {
	reader, ok := blobs.(interface {
		ReadSnapshot(context.Context, string) (io.ReadCloser, error)
	})
	if !ok {
		return nil, errors.New("object storage cannot read publish snapshots")
	}
	return reader.ReadSnapshot(ctx, key)
}

// buildProductionDependencies migrates PostgreSQL and assembles production
// adapters, closing the database on setup failure or returning its cleanup owner.
func buildProductionDependencies(
	ctx context.Context,
	settings config.Config,
	factory productionFactory,
) (ServiceDependencies, func() error, error) {
	if settings.Environment != "production" {
		return ServiceDependencies{}, nil, errors.New("production environment is required")
	}
	database, err := factory.openDatabase(ctx, settings.DatabaseURL)
	if err != nil {
		return ServiceDependencies{}, nil, fmt.Errorf("open production database: %w", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = database.Close()
		}
	}()
	blobs, err := factory.newBlobStore(providerS3.Config{
		Endpoint: settings.S3Endpoint, PublicBaseURL: settings.MediaPublicBaseURL,
		Region: settings.S3Region, Bucket: settings.S3Bucket,
		AccessKeyID: settings.S3AccessKeyID, SecretAccessKey: settings.S3SecretAccessKey,
		SessionToken: settings.S3SessionToken, UsePathStyle: settings.S3UsePathStyle, RequireHTTPS: true,
	})
	if err != nil {
		return ServiceDependencies{}, nil, fmt.Errorf("configure production object storage: %w", err)
	}
	if err := postgres.Migrate(ctx, database, postgres.MigrationOptions{
		ResolveAssetSourceURL: blobs.PublicURL,
	}); err != nil {
		return ServiceDependencies{}, nil, fmt.Errorf("migrate production database: %w", err)
	}
	pagesClient, err := factory.newPages(pages.Config{
		Token: settings.PagesAPIToken, Region: settings.PagesRegion,
	})
	if err != nil {
		return ServiceDependencies{}, nil, fmt.Errorf("configure Pages client: %w", err)
	}
	generator, err := factory.newGenerator()
	if err != nil {
		return ServiceDependencies{}, nil, fmt.Errorf("configure embedded site generator: %w", err)
	}
	adminFiles, err := factory.newAdmin()
	if err != nil {
		return ServiceDependencies{}, nil, fmt.Errorf("configure embedded management pages: %w", err)
	}
	if _, err := adminweb.NewHandler(adminFiles, http.NotFoundHandler()); err != nil {
		return ServiceDependencies{}, nil, err
	}
	repository := postgres.NewPublisherRepository(database, database)
	if err := repository.Health(ctx); err != nil {
		return ServiceDependencies{}, nil, fmt.Errorf("verify publisher storage: %w", err)
	}
	if factory.readSnapshot == nil {
		return ServiceDependencies{}, nil, errors.New("snapshot reader is required")
	}
	worker, err := publisher.NewService(publisher.Options{
		Repository: repository, Pages: pagesClient, Generate: generator.Generate,
		ReadSnapshot: func(ctx context.Context, key string) (io.ReadCloser, error) {
			return factory.readSnapshot(ctx, blobs, key)
		},
		Target: publisher.Target{ProjectID: settings.PagesProjectID, Region: settings.PagesRegion, PublicURL: settings.PagesPublicURL, CodeRevision: generator.Identity()},
	})
	if err != nil {
		return ServiceDependencies{}, nil, fmt.Errorf("configure built-in publisher: %w", err)
	}
	validator := contract.NewValidator()
	publishService := publish.NewService(publish.ServiceOptions{
		Repository: postgres.NewPublishRepository(database), BlobStore: blobs,
		BuildTrigger: worker, Validator: validator,
	})
	dependencies := ServiceDependencies{
		Sessions: postgres.NewSessionStore(database),
		Content: content.NewService(content.ServiceOptions{
			Store: postgres.NewContentRepository(database), Validator: validator,
		}),
		Assets: assets.NewService(assets.ServiceOptions{
			Repository: postgres.NewAssetRepository(database), BlobStore: blobs,
		}),
		Publish:             publishService,
		PublishReconciler:   publishService,
		PublisherReconciler: worker,
		Publisher:           worker,
		AdminFiles:          adminFiles,
	}
	closeOnError = false
	return dependencies, database.Close, nil
}

// buildHandler wires authentication, exact-origin CORS and the shared services.
// Production rejects incomplete dependencies instead of using development state.
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
	sessions, err := auth.NewSessions(auth.SessionOptions{
		Issuer: settings.OIDCIssuer, ClientID: settings.AdminOIDCClientID, ClientSecret: settings.AdminOIDCClientSecret,
		RedirectURL: settings.AdminOIDCRedirectURL, ReturnURL: settings.AdminReturnURL, Key: settings.AdminSessionKey,
		Store: dependencies.Sessions, AllowedOrigins: settings.AllowedAdminOrigins, Secure: settings.Environment == "production",
	})
	if err != nil {
		return nil, err
	}
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{
		Environment:      settings.Environment,
		AllowDevIdentity: settings.AllowDevIdentity,
		IdentityProvider: identityProvider,
		Sessions:         sessions,
	})
	if err != nil {
		return nil, err
	}
	var recovery httpapi.PublisherService
	if dependencies.Publisher != nil {
		recovery = dependencies.Publisher
	}
	api := httpapi.NewRouter(httpapi.RouterOptions{
		Content:        dependencies.Content,
		Assets:         dependencies.Assets,
		Publish:        dependencies.Publish,
		Middleware:     middleware,
		AllowedOrigins: settings.AllowedAdminOrigins,
		LocalUploads:   dependencies.LocalUploads,
		Sessions:       sessions,
		Publisher:      recovery,
	})
	adminFiles := dependencies.AdminFiles
	if adminFiles == nil {
		adminFiles, err = webassets.Admin()
		if err != nil {
			if settings.Environment == "production" {
				return nil, err
			}
			return api, nil
		}
	}
	return adminweb.NewHandler(adminFiles, api)
}

// developmentDependencies shares one in-memory state and temporary blob store
// across development services; the caller must invoke Close to remove files.
func developmentDependencies() ServiceDependencies {
	state := memory.NewState()
	validator := contract.NewValidator()
	blobs := local.NewBlobStore()
	trigger := local.NewBuildTrigger()
	publishService := publish.NewService(publish.ServiceOptions{
		Repository:   memory.NewPublishRepository(state),
		BlobStore:    blobs,
		BuildTrigger: trigger,
		Validator:    validator,
	})
	return ServiceDependencies{
		Sessions: memory.NewSessionStore(),
		Content: content.NewService(content.ServiceOptions{
			Store:     memory.NewContentRepository(state),
			Validator: validator,
		}),
		Assets: assets.NewService(assets.ServiceOptions{
			Repository: memory.NewAssetRepository(state),
			BlobStore:  blobs,
		}),
		Publish:           publishService,
		PublishReconciler: publishService,
		LocalUploads:      blobs,
		Close:             blobs.Close,
	}
}

// runPublishReconciler reconciles immediately, then waits between completed
// runs; cancellation stops the loop and suppresses expected shutdown errors.
func runPublishReconciler(ctx context.Context, interval time.Duration, reconciler publishReconciler, logger *slog.Logger) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	for {
		if err := reconciler.Reconcile(ctx); err != nil && ctx.Err() == nil {
			logger.Error("reconcile publish job", "error", err)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

// runServer drains requests on cancellation, then disconnects remaining clients
// on timeout so dependency cleanup cannot wait for extended upload deadlines.
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
			return errors.Join(err, server.Close())
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
