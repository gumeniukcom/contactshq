package handler

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gumeniukcom/contactshq/internal/service"
)

type ContactHandler struct {
	contactService *service.ContactService
}

func NewContactHandler(contactService *service.ContactService) *ContactHandler {
	return &ContactHandler{contactService: contactService}
}

func (h *ContactHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	query := c.Query("q")

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var contacts interface{}
	var total int
	var err error

	if query != "" {
		contacts, total, err = h.contactService.Search(c.Context(), userID, query, limit, offset)
	} else {
		contacts, total, err = h.contactService.List(c.Context(), userID, limit, offset)
	}

	if err != nil {
		if errors.Is(err, service.ErrAddressBookNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "address book not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list contacts"})
	}

	return c.JSON(fiber.Map{
		"contacts": contacts,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

func (h *ContactHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	var input service.CreateContactInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	contact, err := h.contactService.Create(c.Context(), userID, input)
	if err != nil {
		if errors.Is(err, service.ErrAddressBookNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "address book not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create contact"})
	}

	return c.Status(fiber.StatusCreated).JSON(contact)
}

func (h *ContactHandler) Get(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	contactID := c.Params("id")

	contact, err := h.contactService.GetByID(c.Context(), userID, contactID)
	if err != nil {
		if errors.Is(err, service.ErrContactNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "contact not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get contact"})
	}

	return c.JSON(contact)
}

func (h *ContactHandler) Update(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	contactID := c.Params("id")

	var input service.UpdateContactInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	contact, err := h.contactService.Update(c.Context(), userID, contactID, input)
	if err != nil {
		if errors.Is(err, service.ErrContactNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "contact not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update contact"})
	}

	return c.JSON(contact)
}

func (h *ContactHandler) Delete(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	contactID := c.Params("id")

	err := h.contactService.Delete(c.Context(), userID, contactID)
	if err != nil {
		if errors.Is(err, service.ErrContactNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "contact not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete contact"})
	}

	return c.JSON(fiber.Map{"message": "contact deleted"})
}

func (h *ContactHandler) DeleteAll(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	if err := h.contactService.DeleteAll(c.Context(), userID); err != nil {
		if errors.Is(err, service.ErrAddressBookNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "address book not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete contacts"})
	}

	return c.JSON(fiber.Map{"message": "all contacts deleted"})
}

func (h *ContactHandler) GetVCard(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)
	contactID := c.Params("id")

	contact, err := h.contactService.GetByID(c.Context(), userID, contactID)
	if err != nil {
		if errors.Is(err, service.ErrContactNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "contact not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get contact"})
	}

	c.Set("Content-Type", "text/vcard; charset=utf-8")
	c.Set("Content-Disposition", "attachment; filename=\""+contact.UID+".vcf\"")
	return c.SendString(contact.VCardData)
}
