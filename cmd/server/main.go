package main

import (
	"context"
	"errors"
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
	// Subcommands are dispatched before the server's own configuration is read: a
	// `set-password` run must work on a deployment whose CHQ_AUTH_JWT_SECRET the operator
	// does not have to hand, which is exactly the situation locked-out people are in.
	if handled, code := runCLI(os.Args, os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}

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

	// Recorded before anything else can open a new run, so "started before this process" is
	// a clean cut.
	processStartedAt := time.Now()

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
	syncCursorRepo := repository.NewBunSyncCursorRepository(db)

	// Services
	authService := service.NewAuthService(userRepo, abRepo, cfg.Auth)
	userService := service.NewUserService(userRepo)
	contactService := service.NewContactService(contactRepo, abRepo)
	importerService := service.NewImporterService(contactRepo, abRepo)
	exporterService := service.NewExporterService(contactRepo, abRepo)
	qrcodeService := service.NewQRCodeService()
	pipelineService := service.NewPipelineService(pipelineRepo)
	backupRunRepo := repository.NewBunBackupRunRepository(db)
	backupService := service.NewBackupService(contactRepo, abRepo, backupSettingsRepo, logger, cfg.Backup.Dir, cfg.Backup.Schedule, 7).
		WithSyncStateRepo(syncRepo).
		WithRunRepo(backupRunRepo).
		WithMaxRestoreBytes(cfg.Backup.MaxRestoreBytes)
	dupDetector := service.NewDuplicateDetector(contactRepo, abRepo, dupRepo, logger)
	mergeLogRepo := repository.NewBunMergeLogRepository(db)
	mergeService := service.NewMergeService(contactRepo, abRepo, dupRepo, syncRepo).
		WithMergeLog(mergeLogRepo).
		WithLogger(logger)

	appPwService := service.NewAppPasswordService(appPwRepo)
	syncConflictService := service.NewSyncConflictService(syncConflictRepo, syncRepo, contactRepo, abRepo)

	// Google OAuth
	googleOAuth := service.NewGoogleOAuthService(cfg.Google, providerConnRepo)

	// Sync engine & pipeline orchestrator
	syncEngine := chqsync.NewEngineWithAllRepos(syncRepo, syncRunRepo, syncConflictRepo, logger).
		WithCursorStore(syncCursorRepo)
	orchestrator := chqsync.NewPipelineOrchestrator(syncEngine, contactRepo, abRepo, pipelineRepo, providerConnRepo, googleOAuth, logger)

	// Worker
	gWorker := worker.NewGoroutineWorker(4, logger)
	gWorker.Register("pipeline", jobs.NewPipelineJobHandler(orchestrator, pipelineRepo, logger).Handle)
	gWorker.Register("backup", jobs.NewBackupJobHandler(backupService, logger).Handle)
	gWorker.Register("sync", jobs.NewSyncJobHandler(syncEngine, contactRepo, abRepo, providerConnRepo, logger).Handle)
	gWorker.Register("dedup", jobs.NewDedupJobHandler(dupDetector, logger).
		WithMergeLogRetention(mergeLogRepo, cfg.Merge.LogRetentionDays).Handle)
	// Once at startup as well: an instance with no dedup schedule would otherwise never
	// prune the merge history.
	jobs.PruneMergeLog(ctx, mergeLogRepo, cfg.Merge.LogRetentionDays, logger)
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

	// History left open by a process that died, closed once at boot.
	reconcileInterruptedRuns(ctx, backupRunRepo, syncRunRepo, processStartedAt, logger)
	pruneSyncRuns(ctx, syncRunRepo, cfg.Sync.RunsRetentionDays, logger)

	// A container killed overnight means the scheduled backup never happened and the next
	// firing is a day away. This is the cheap half of a durable queue: the one job whose loss
	// actually costs something gets a second chance at boot.
	catchUpMissedBackups(ctx, userIDs, backupService, backupRunRepo, gWorker, logger)

	// Fiber app.
	//
	// Fiber only routes methods listed in RequestMethods and answers 400 to anything
	// else, so the CardDAV verbs must be registered here or the /dav mount is
	// unreachable to every client.
	fiberCfg := fiber.Config{
		AppName:        "ContactsHQ",
		BodyLimit:      10 * 1024 * 1024, // 10MB
		ErrorHandler:   newErrorHandler(logger),
		RequestMethods: append(fiber.DefaultMethods, webdavMethods...),
	}
	// Only believe X-Forwarded-For when the request actually came through a configured
	// proxy. Without this the header is spoofable, so it stays off — and c.IP(), which
	// keys the auth rate limiter, is the direct peer — unless trusted proxies are set.
	if len(cfg.Server.TrustedProxies) > 0 {
		fiberCfg.ProxyHeader = fiber.HeaderXForwardedFor
		fiberCfg.EnableTrustedProxyCheck = true
		fiberCfg.TrustedProxies = cfg.Server.TrustedProxies
		fiberCfg.EnableIPValidation = true // return the first valid IP, not the raw header
		logger.Info("trusting X-Forwarded-For from configured proxies",
			zap.Strings("trusted_proxies", cfg.Server.TrustedProxies))
	}
	app := fiber.New(fiberCfg)

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
		MergeLogRepo:      mergeLogRepo,
		BackupRunRepo:     backupRunRepo,
		DupDetector:       dupDetector,
		MergeService:      mergeService,
		DedupSettingsRepo: dedupSettingsRepo,
		Scheduler:         sched,
		GoogleOAuth:       googleOAuth,
		AppPassword:       appPwService,
		SyncConflict:      syncConflictService,
		DB:                db,
		Logger:            logger,
	})

	// RFC 6764 — CardDAV service discovery
	davPrefix := cfg.CardDAV.PathPrefix
	app.Get("/.well-known/carddav", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "max-age=86400")
		return c.Redirect(davPrefix+"/", fiber.StatusMovedPermanently)
	})

	// CardDAV server
	davBackend := chqcarddav.NewBackend(userRepo, abRepo, contactRepo, davPrefix)
	// The same proxy list Fiber uses: /dav is mounted through adaptor.HTTPHandler and sees a
	// net/http request, so Fiber's own trusted-proxy handling never reaches it.
	davServer := chqcarddav.NewServerWithTrustedProxies(
		davBackend, userRepo, appPwRepo, davPrefix, cfg.Server.TrustedProxies)

	// Wired here rather than at construction because the CardDAV server is built last. The
	// services hold the callback, so a changed password or a deleted app password stops
	// opening CardDAV immediately instead of after the verdict cache expires.
	userService.WithCredentialInvalidator(davServer.InvalidateUser)
	appPwService.WithCredentialInvalidator(userRepo, davServer.InvalidateUser)
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

// newErrorHandler renders an error to the client without disclosing what it was.
//
// A *fiber.Error carries a message this application chose, so it is safe to return. Anything
// else is an internal failure — a driver error, a panic recovered by the framework — whose
// text may name tables, paths or hosts. Those go to the log; the client gets a fixed string
// and the form of the body stays {"error": "..."} because web/src/api/client.ts reads it.
func newErrorHandler(logger *zap.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		var fe *fiber.Error
		if errors.As(err, &fe) {
			return c.Status(fe.Code).JSON(fiber.Map{"error": fe.Message})
		}

		fields := []zap.Field{
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Error(err),
		}
		// Full request-id plumbing is a separate task; honour one if a proxy supplied it.
		if rid := c.Get("X-Request-ID"); rid != "" {
			fields = append(fields, zap.String("request_id", rid))
		}
		logger.Error("unhandled request error", fields...)

		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": "internal server error"})
	}
}
