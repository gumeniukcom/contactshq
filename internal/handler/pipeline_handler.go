package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/repository"
	"github.com/gumeniukcom/contactshq/internal/service"
	chqsync "github.com/gumeniukcom/contactshq/internal/sync"
)

type PipelineHandler struct {
	pipelineService *service.PipelineService
	orchestrator    *chqsync.PipelineOrchestrator
	syncRunRepo     repository.SyncRunRepository
}

func NewPipelineHandler(pipelineService *service.PipelineService, orchestrator *chqsync.PipelineOrchestrator, syncRunRepo repository.SyncRunRepository) *PipelineHandler {
	return &PipelineHandler{
		pipelineService: pipelineService,
		orchestrator:    orchestrator,
		syncRunRepo:     syncRunRepo,
	}
}

func (h *PipelineHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	pipelines, err := h.pipelineService.List(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list pipelines"})
	}

	return c.JSON(fiber.Map{"pipelines": pipelines})
}

func (h *PipelineHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var input service.CreatePipelineInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if input.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	pipeline, err := h.pipelineService.Create(c.Context(), userID, input)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create pipeline"})
	}

	return c.Status(fiber.StatusCreated).JSON(pipeline)
}

func (h *PipelineHandler) Get(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	pipelineID := c.Params("id")

	pipeline, err := h.pipelineService.GetByID(c.Context(), userID, pipelineID)
	if err != nil {
		if errors.Is(err, service.ErrPipelineNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "pipeline not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get pipeline"})
	}

	return c.JSON(pipeline)
}

func (h *PipelineHandler) Update(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	pipelineID := c.Params("id")

	var input service.CreatePipelineInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	pipeline, err := h.pipelineService.Update(c.Context(), userID, pipelineID, input)
	if err != nil {
		if errors.Is(err, service.ErrPipelineNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "pipeline not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update pipeline"})
	}

	return c.JSON(pipeline)
}

func (h *PipelineHandler) Delete(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	pipelineID := c.Params("id")

	err := h.pipelineService.Delete(c.Context(), userID, pipelineID)
	if err != nil {
		if errors.Is(err, service.ErrPipelineNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "pipeline not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete pipeline"})
	}

	return c.JSON(fiber.Map{"message": "pipeline deleted"})
}

func (h *PipelineHandler) Trigger(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	pipelineID := c.Params("id")

	pipeline, err := h.pipelineService.GetByID(c.Context(), userID, pipelineID)
	if err != nil {
		if errors.Is(err, service.ErrPipelineNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "pipeline not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get pipeline"})
	}

	results, err := h.orchestrator.Execute(c.Context(), userID, pipeline)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "pipeline execution failed"})
	}

	return c.JSON(fiber.Map{
		"message": "pipeline executed",
		"results": results,
	})
}

func (h *PipelineHandler) ListRuns(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	pipelineID := c.Params("id")
	limit := c.QueryInt("limit", 50)
	if limit > 200 {
		limit = 200
	}

	runs, err := h.syncRunRepo.ListByPipeline(c.Context(), userID, pipelineID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list runs"})
	}

	return c.JSON(fiber.Map{"runs": runs})
}
