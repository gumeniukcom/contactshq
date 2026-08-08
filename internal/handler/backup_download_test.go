package handler_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"

	"github.com/gumeniukcom/contactshq/internal/handler"
	"github.com/gumeniukcom/contactshq/internal/service"
)

// A backup download has to name the file it is sending. An API client — `curl -OJ`, a backup
// script, anything that is not the SPA — takes the filename from Content-Disposition and
// otherwise invents one from the URL path.
func TestBackupDownload_SetsAnAttachmentFilename(t *testing.T) {
	const userID = "user-1"
	const name = "backup-20260807-101112-123.vcf.gz"

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, userID), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, userID, name), []byte("backup bytes"), 0o600))

	// GetPath is the only part of the service this route touches, and it reads the filesystem
	// alone — the repositories stay nil deliberately.
	h := handler.NewBackupHandler(service.NewBackupService(nil, nil, nil, nil, dir, "", 7), nil, nil)

	app := fiber.New()
	app.Get("/backup/download/:id", func(c *fiber.Ctx) error {
		c.Locals("userID", userID)
		return h.Download(c)
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/backup/download/"+name, nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, `attachment; filename="`+name+`"`, resp.Header.Get("Content-Disposition"))
}
