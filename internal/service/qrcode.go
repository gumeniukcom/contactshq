package service

import (
	"fmt"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/gumeniukcom/contactshq/internal/domain"
)

type QRCodeService struct{}

func NewQRCodeService() *QRCodeService {
	return &QRCodeService{}
}

func (s *QRCodeService) GenerateVCardQR(contact *domain.Contact, size int) ([]byte, error) {
	if size <= 0 {
		size = 256
	}

	mecard := fmt.Sprintf("MECARD:N:%s,%s;TEL:%s;EMAIL:%s;ORG:%s;;",
		contact.LastName, contact.FirstName, contact.Phone, contact.Email, contact.Org)

	return qrcode.Encode(mecard, qrcode.Medium, size)
}
