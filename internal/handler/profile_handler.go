package handler

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ProfileHandler struct{}

func NewProfileHandler() *ProfileHandler {
	return &ProfileHandler{}
}

func (h *ProfileHandler) IOSProfile(c *fiber.Ctx) error {
	email := c.Locals("email").(string)

	host := c.Get("X-Forwarded-Host")
	if host == "" {
		host = c.Hostname()
	}

	useSSL := true
	port := 443
	proto := c.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = c.Protocol()
	}
	if proto != "https" {
		useSSL = false
		port = 80
	}

	payloadUUID := uuid.New().String()
	profileUUID := uuid.New().String()

	sslString := "false"
	if useSSL {
		sslString = "true"
	}

	// Safe XML escaping for email/host
	safeEmail := xmlEscape(email)
	safeHost := xmlEscape(host)

	profile := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>CardDAVAccountDescription</key>
			<string>ContactsHQ Contacts</string>
			<key>CardDAVHostName</key>
			<string>%s</string>
			<key>CardDAVPort</key>
			<integer>%d</integer>
			<key>CardDAVPrincipalURL</key>
			<string>/dav/%s/</string>
			<key>CardDAVUseSSL</key>
			<%s/>
			<key>CardDAVUsername</key>
			<string>%s</string>
			<key>PayloadDescription</key>
			<string>Syncs contacts with ContactsHQ</string>
			<key>PayloadDisplayName</key>
			<string>ContactsHQ Contacts</string>
			<key>PayloadIdentifier</key>
			<string>com.contactshq.carddav.%s</string>
			<key>PayloadType</key>
			<string>com.apple.carddav.account</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
		</dict>
	</array>
	<key>PayloadDescription</key>
	<string>Configures CardDAV contact sync with ContactsHQ</string>
	<key>PayloadDisplayName</key>
	<string>ContactsHQ</string>
	<key>PayloadIdentifier</key>
	<string>com.contactshq.profile.%s</string>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>%s</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>`, safeHost, port, safeEmail, sslString, safeEmail, safeEmail, payloadUUID, safeEmail, profileUUID)

	c.Set("Content-Type", "application/x-apple-aspen-config")
	c.Set("Content-Disposition", `attachment; filename="ContactsHQ.mobileconfig"`)
	return c.SendString(profile)
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}
