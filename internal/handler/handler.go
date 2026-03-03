package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/handler/middleware"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
	"github.com/gumeniukcom/contactshq/internal/worker"
)

type Services struct {
	Version          string
	BuildTime        string
	Auth             *service.AuthService
	User             *service.UserService
	Contact          *service.ContactService
	Importer         *service.ImporterService
	Exporter         *service.ExporterService
	QRCode           *service.QRCodeService
	Pipeline         *service.PipelineService
	Backup           *service.BackupService
	Orchestrator     *chqsync.PipelineOrchestrator
	Worker           worker.TaskWorker
	SyncRunRepo      repository.SyncRunRepository
	SyncStateRepo    repository.SyncStateRepository
	SyncConflictRepo repository.SyncConflictRepository
	ProviderConnRepo repository.ProviderConnectionRepository
	DupRepo          repository.PotentialDuplicateRepository
	DupDetector      *service.DuplicateDetector
	MergeService     *service.MergeService
	Scheduler        *worker.Scheduler
}

func Register(app *fiber.App, svc Services) {
	authHandler := NewAuthHandler(svc.Auth)
	userHandler := NewUserHandler(svc.User)
	contactHandler := NewContactHandler(svc.Contact)
	adminHandler := NewAdminHandler(svc.User)
	syncHandler := NewSyncHandler(svc.SyncRunRepo, svc.SyncStateRepo, svc.SyncConflictRepo, svc.ProviderConnRepo, svc.Worker)
	credHandler := NewCredentialHandler(svc.ProviderConnRepo)

	api := app.Group("/api/v1")

	// Auth (public)
	auth := api.Group("/auth")
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)
	auth.Post("/refresh", authHandler.Refresh)

	// Protected routes
	protected := api.Use(middleware.JWTAuth(svc.Auth))

	// User
	users := protected.Group("/users")
	users.Get("/me", userHandler.GetMe)
	users.Put("/me", userHandler.UpdateMe)
	users.Put("/me/password", userHandler.ChangePassword)
	users.Delete("/me", userHandler.DeleteMe)

	// Contacts — static sub-paths must come before /:id
	contacts := protected.Group("/contacts")
	contacts.Get("/", contactHandler.List)
	contacts.Post("/", contactHandler.Create)
	contacts.Delete("/", contactHandler.DeleteAll)

	// Duplicate detection & merge (before /:id to avoid shadowing)
	if svc.DupDetector != nil && svc.MergeService != nil && svc.DupRepo != nil {
		dupHandler := NewDuplicateHandler(svc.DupDetector, svc.MergeService, svc.DupRepo)
		contacts.Get("/duplicates", dupHandler.List)
		contacts.Get("/duplicates/count", dupHandler.Count)
		contacts.Post("/duplicates/detect", dupHandler.Detect)
		contacts.Post("/duplicates/:id/dismiss", dupHandler.Dismiss)
		contacts.Post("/merge", dupHandler.Merge)
	}

	contacts.Get("/:id", contactHandler.Get)
	contacts.Put("/:id", contactHandler.Update)
	contacts.Delete("/:id", contactHandler.Delete)
	contacts.Get("/:id/vcard", contactHandler.GetVCard)

	// Import/Export
	if svc.Importer != nil {
		importHandler := NewImportHandler(svc.Importer)
		imp := protected.Group("/import")
		imp.Post("/vcard", importHandler.ImportVCard)
		imp.Post("/csv", importHandler.ImportCSV)
	}

	if svc.Exporter != nil {
		exportHandler := NewExportHandler(svc.Exporter)
		exp := protected.Group("/export")
		exp.Get("/vcard", exportHandler.ExportVCard)
		exp.Get("/csv", exportHandler.ExportCSV)
		exp.Get("/json", exportHandler.ExportJSON)
	}

	if svc.QRCode != nil {
		qrHandler := NewQRCodeHandler(svc.QRCode, svc.Contact)
		contacts.Get("/:id/qrcode", qrHandler.GenerateQR)
	}

	// Credentials (connection vault)
	creds := protected.Group("/credentials")
	creds.Get("/", credHandler.List)
	creds.Post("/", credHandler.Create)
	creds.Get("/:id", credHandler.Get)
	creds.Put("/:id", credHandler.Update)
	creds.Delete("/:id", credHandler.Delete)

	// Sync
	syncGroup := protected.Group("/sync")
	syncGroup.Get("/providers", syncHandler.ListProviders)
	syncGroup.Post("/google/connect", syncHandler.GoogleConnect)
	syncGroup.Post("/google/trigger", syncHandler.GoogleTrigger)
	syncGroup.Post("/carddav/connect", syncHandler.CardDAVConnect)
	syncGroup.Post("/carddav/trigger", syncHandler.CardDAVTrigger)
	syncGroup.Delete("/providers/:id", syncHandler.DisconnectProvider)
	syncGroup.Get("/status", syncHandler.Status)
	syncGroup.Get("/history", syncHandler.History)
	syncGroup.Get("/conflicts", syncHandler.ListConflicts)
	syncGroup.Get("/conflicts/count", syncHandler.CountConflicts)
	syncGroup.Get("/conflicts/:id", syncHandler.GetConflict)
	syncGroup.Post("/conflicts/:id/resolve", syncHandler.ResolveConflict)
	syncGroup.Post("/conflicts/:id/dismiss", syncHandler.DismissConflict)

	// Pipelines
	if svc.Pipeline != nil && svc.Orchestrator != nil {
		pipelineHandler := NewPipelineHandler(svc.Pipeline, svc.Orchestrator, svc.SyncRunRepo)
		pipelines := protected.Group("/pipelines")
		pipelines.Get("/", pipelineHandler.List)
		pipelines.Post("/", pipelineHandler.Create)
		pipelines.Get("/:id", pipelineHandler.Get)
		pipelines.Put("/:id", pipelineHandler.Update)
		pipelines.Delete("/:id", pipelineHandler.Delete)
		pipelines.Post("/:id/trigger", pipelineHandler.Trigger)
		pipelines.Get("/:id/runs", pipelineHandler.ListRuns)
	}

	// Backup
	if svc.Backup != nil {
		backupHandler := NewBackupHandler(svc.Backup, svc.Scheduler)
		backup := protected.Group("/backup")
		backup.Post("/create", backupHandler.Create)
		backup.Get("/list", backupHandler.List)
		backup.Get("/settings", backupHandler.GetSettings)
		backup.Put("/settings", backupHandler.SaveSettings)
		backup.Get("/download/:id", backupHandler.Download)
		backup.Delete("/:id", backupHandler.Delete)
		backup.Post("/restore/:id", backupHandler.Restore)
	}

	// Admin
	admin := protected.Group("/admin", middleware.AdminOnly())
	admin.Get("/users", adminHandler.ListUsers)
	admin.Post("/users", authHandler.Register)
	admin.Put("/users/:id/role", adminHandler.UpdateUserRole)
	admin.Delete("/users/:id", adminHandler.DeleteUser)

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":     "ok",
			"version":    svc.Version,
			"build_time": svc.BuildTime,
		})
	})
}
