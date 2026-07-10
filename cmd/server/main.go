package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	chqcarddav "github.com/gumeniukcom/contactshq/internal/carddav"
	"github.com/gumeniukcom/contactshq/internal/config"
	"github.com/gumeniukcom/contactshq/internal/handler"
	"github.com/gumeniukcom/contactshq/internal/handler/middleware"
	chqlogger "github.com/gumeniukcom/contactshq/internal/logger"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
	chqweb "github.com/gumeniukcom/contactshq/internal/web"
	"github.com/gumeniukcom/contactshq/internal/worker"
	"github.com/gumeniukcom/contactshq/internal/worker/jobs"
	"go.uber.org/zap"
)

// Version and BuildTime are injected at build time via -ldflags.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

// shutdownTimeout bounds the drain of HTTP requests, scheduler and job queue.
const shutdownTimeout = 30 * time.Second

// webdavMethods are the RFC 4918 / RFC 6352 verbs the CardDAV server answers, none of
// which are part of Fiber's default method set.
var webdavMethods = []string{
	"PROPFIND",
	"PROPPATCH",
	"REPORT",
	"MKCOL",
	"COPY",
	"MOVE",
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger, err := chqlogger.New(cfg.Log)
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}
	defer func() { _ = logger.Sync() }()

	db, err := repository.NewDB(cfg.Database)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	logDatabaseLocation(logger, cfg.Database)

	ctx := context.Background()
	if err := repository.Migrate(ctx, db); err != nil {
		logger.Fatal("failed to run migrations", zap.Error(err))
	}

	// Repositories
	userRepo := repository.NewBunUserRepository(db)
	abRepo := repository.NewBunAddressBookRepository(db)
	contactRepo := repository.NewBunContactRepository(db)
	syncRepo := repository.NewBunSyncStateRepository(db)
	syncRunRepo := repository.NewBunSyncRunRepository(db)
	syncConflictRepo := repository.NewBunSyncConflictRepository(db)
	dupRepo := repository.NewBunPotentialDuplicateRepository(db)
	pipelineRepo := repository.NewBunPipelineRepository(db)
	backupSettingsRepo := repository.NewBunUserBackupSettingsRepository(db)
	dedupSettingsRepo := repository.NewBunUserDedupSettingsRepository(db)
	providerConnRepo := repository.NewBunProviderConnectionRepository(db)
	appPwRepo := repository.NewBunAppPasswordRepository(db)

	// Services
	authService := service.NewAuthService(userRepo, abRepo, cfg.Auth)
	userService := service.NewUserService(userRepo)
	contactService := service.NewContactService(contactRepo, abRepo)
	importerService := service.NewImporterService(contactRepo, abRepo)
	exporterService := service.NewExporterService(contactRepo, abRepo)
	qrcodeService := service.NewQRCodeService()
	pipelineService := service.NewPipelineService(pipelineRepo)
	backupService := service.NewBackupService(contactRepo, abRepo, backupSettingsRepo, cfg.Backup.Dir, cfg.Backup.Schedule, 7).
		WithSyncStateRepo(syncRepo)
	dupDetector := service.NewDuplicateDetector(contactRepo, abRepo, dupRepo, logger)
	mergeService := service.NewMergeService(contactRepo, abRepo, dupRepo, syncRepo)

	appPwService := service.NewAppPasswordService(appPwRepo)
	syncConflictService := service.NewSyncConflictService(syncConflictRepo, syncRepo, contactRepo, abRepo)

	// Google OAuth
	googleOAuth := service.NewGoogleOAuthService(cfg.Google, providerConnRepo)

	// Sync engine & pipeline orchestrator
	syncEngine := chqsync.NewEngineWithAllRepos(syncRepo, syncRunRepo, syncConflictRepo, logger)
	orchestrator := chqsync.NewPipelineOrchestrator(syncEngine, contactRepo, abRepo, pipelineRepo, providerConnRepo, googleOAuth, logger)

	// Worker
	gWorker := worker.NewGoroutineWorker(4, logger)
	gWorker.Register("pipeline", jobs.NewPipelineJobHandler(orchestrator, pipelineRepo, logger).Handle)
	gWorker.Register("backup", jobs.NewBackupJobHandler(backupService, logger).Handle)
	gWorker.Register("sync", jobs.NewSyncJobHandler(syncEngine, contactRepo, abRepo, providerConnRepo, logger).Handle)
	gWorker.Register("dedup", jobs.NewDedupJobHandler(dupDetector, logger).Handle)
	if err := gWorker.Start(ctx); err != nil {
		logger.Fatal("failed to start worker", zap.Error(err))
	}

	// Scheduler
	sched, err := worker.NewScheduler(gWorker, logger)
	if err != nil {
		logger.Fatal("failed to create scheduler", zap.Error(err))
	}

	pipelines, err := pipelineRepo.ListAllEnabled(ctx)
	if err != nil {
		logger.Warn("failed to load enabled pipelines for scheduler", zap.Error(err))
	} else {
		sched.RegisterPipelines(ctx, pipelines)
	}

	userIDs, err := userRepo.ListAllIDs(ctx)
	if err != nil {
		logger.Warn("failed to load user IDs for backup scheduler", zap.Error(err))
	} else {
		for _, uid := range userIDs {
			schedule, err := backupService.GetUserSchedule(ctx, uid)
			if err != nil {
				logger.Warn("failed to get backup schedule for user", zap.String("user_id", uid), zap.Error(err))
				continue
			}
			if schedule != "" {
				sched.RegisterBackupForUser(schedule, uid)
			}
		}
	}
	// Load dedup schedules
	dedupSettings, err := dedupSettingsRepo.ListAll(ctx)
	if err != nil {
		logger.Warn("failed to load dedup settings for scheduler", zap.Error(err))
	} else {
		for _, ds := range dedupSettings {
			if ds.Enabled && ds.Schedule != "" {
				sched.RegisterDedupForUser(ds.Schedule, ds.UserID)
			}
		}
	}

	sched.Start()

	// Fiber app.
	//
	// Fiber only routes methods listed in RequestMethods and answers 400 to anything
	// else, so the CardDAV verbs must be registered here or the /dav mount is
	// unreachable to every client.
	app := fiber.New(fiber.Config{
		AppName:        "ContactsHQ",
		BodyLimit:      10 * 1024 * 1024, // 10MB
		ErrorHandler:   errorHandler,
		RequestMethods: append(fiber.DefaultMethods, webdavMethods...),
	})

	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(middleware.RequestLogger(logger))

	handler.Register(app, handler.Services{
		Version:           Version,
		BuildTime:         BuildTime,
		Auth:              authService,
		User:              userService,
		Contact:           contactService,
		Importer:          importerService,
		Exporter:          exporterService,
		QRCode:            qrcodeService,
		Pipeline:          pipelineService,
		Backup:            backupService,
		Orchestrator:      orchestrator,
		Worker:            gWorker,
		SyncRunRepo:       syncRunRepo,
		SyncStateRepo:     syncRepo,
		SyncConflictRepo:  syncConflictRepo,
		ProviderConnRepo:  providerConnRepo,
		DupRepo:           dupRepo,
		DupDetector:       dupDetector,
		MergeService:      mergeService,
		DedupSettingsRepo: dedupSettingsRepo,
		Scheduler:         sched,
		GoogleOAuth:       googleOAuth,
		AppPassword:       appPwService,
		SyncConflict:      syncConflictService,
		DB:                db,
	})

	// RFC 6764 — CardDAV service discovery
	davPrefix := cfg.CardDAV.PathPrefix
	app.Get("/.well-known/carddav", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "max-age=86400")
		return c.Redirect(davPrefix+"/", fiber.StatusMovedPermanently)
	})

	// CardDAV server
	davBackend := chqcarddav.NewBackend(userRepo, abRepo, contactRepo, davPrefix)
	davServer := chqcarddav.NewServer(davBackend, userRepo, appPwRepo, davPrefix)
	app.Use(davPrefix, adaptor.HTTPHandler(davServer))

	// Web UI (landing + SPA)
	chqweb.RegisterRoutes(app)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	go func() {
		logger.Info("ContactsHQ starting",
			zap.String("addr", addr),
			zap.String("version", Version),
			zap.String("build_time", BuildTime),
		)
		if err := app.Listen(addr); err != nil {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	<-quit
	logger.Info("shutting down")

	// Bound every stage of shutdown. Without a deadline an in-flight sync could hold the
	// process open indefinitely, and the container runtime would eventually kill it in
	// the middle of a write.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error("http shutdown", zap.Error(err))
	}

	sched.Stop()

	if err := gWorker.Stop(shutdownCtx); err != nil {
		logger.Warn("worker did not drain in time", zap.Error(err))
	}

	logger.Info("stopped")
}

// logDatabaseLocation resolves a relative SQLite path so an operator can see which file
// is actually in use. A relative DSN follows the working directory, and an ad-hoc
// container run would otherwise write its database somewhere nobody thinks to look.
func logDatabaseLocation(logger *zap.Logger, cfg config.DatabaseConfig) {
	if cfg.Driver != "sqlite" {
		logger.Info("database", zap.String("driver", cfg.Driver))
		return
	}

	abs, err := filepath.Abs(cfg.DSN)
	if err != nil {
		abs = cfg.DSN
	}
	fields := []zap.Field{zap.String("driver", "sqlite"), zap.String("path", abs)}
	if !filepath.IsAbs(cfg.DSN) {
		fields = append(fields, zap.String("note", "relative path — resolved against the working directory"))
	}
	logger.Info("database", fields...)
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}
