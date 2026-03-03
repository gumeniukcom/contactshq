package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

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

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger, err := chqlogger.New(cfg.Log)
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}
	defer logger.Sync()

	db, err := repository.NewDB(cfg.Database)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

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
	providerConnRepo := repository.NewBunProviderConnectionRepository(db)

	// Services
	authService := service.NewAuthService(userRepo, abRepo, cfg.Auth)
	userService := service.NewUserService(userRepo)
	contactService := service.NewContactService(contactRepo, abRepo)
	importerService := service.NewImporterService(contactRepo, abRepo)
	exporterService := service.NewExporterService(contactRepo, abRepo)
	qrcodeService := service.NewQRCodeService()
	pipelineService := service.NewPipelineService(pipelineRepo)
	backupService := service.NewBackupService(contactRepo, abRepo, backupSettingsRepo, cfg.Backup.Dir, cfg.Backup.Schedule, 7)
	dupDetector := service.NewDuplicateDetector(contactRepo, abRepo, dupRepo, logger)
	mergeService := service.NewMergeService(contactRepo, abRepo, dupRepo, syncRepo)

	// Sync engine & pipeline orchestrator
	syncEngine := chqsync.NewEngineWithAllRepos(syncRepo, syncRunRepo, syncConflictRepo, logger)
	orchestrator := chqsync.NewPipelineOrchestrator(syncEngine, contactRepo, abRepo, pipelineRepo, providerConnRepo, logger)

	// Worker
	gWorker := worker.NewGoroutineWorker(4, logger)
	gWorker.Register("pipeline", jobs.NewPipelineJobHandler(orchestrator, pipelineRepo, logger).Handle)
	gWorker.Register("backup", jobs.NewBackupJobHandler(backupService, logger).Handle)
	gWorker.Register("sync", jobs.NewSyncJobHandler(syncEngine, contactRepo, abRepo, providerConnRepo, logger).Handle)
	if err := gWorker.Start(ctx); err != nil {
		logger.Fatal("failed to start worker", zap.Error(err))
	}
	defer gWorker.Stop(ctx)

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
	sched.Start()
	defer sched.Stop()

	// Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "ContactsHQ",
		BodyLimit:    10 * 1024 * 1024, // 10MB
		ErrorHandler: errorHandler,
	})

	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(middleware.RequestLogger(logger))

	handler.Register(app, handler.Services{
		Version:          Version,
		BuildTime:        BuildTime,
		Auth:             authService,
		User:             userService,
		Contact:          contactService,
		Importer:         importerService,
		Exporter:         exporterService,
		QRCode:           qrcodeService,
		Pipeline:         pipelineService,
		Backup:           backupService,
		Orchestrator:     orchestrator,
		Worker:           gWorker,
		SyncRunRepo:      syncRunRepo,
		SyncStateRepo:    syncRepo,
		SyncConflictRepo: syncConflictRepo,
		ProviderConnRepo: providerConnRepo,
		DupRepo:          dupRepo,
		DupDetector:      dupDetector,
		MergeService:     mergeService,
		Scheduler:        sched,
	})

	// CardDAV server
	davPrefix := cfg.CardDAV.PathPrefix
	davBackend := chqcarddav.NewBackend(userRepo, abRepo, contactRepo, davPrefix)
	davServer := chqcarddav.NewServer(davBackend, userRepo, davPrefix)
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
	_ = app.Shutdown()
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}
