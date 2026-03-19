package handlers

import qrcode "github.com/skip2/go-qrcode"

func generateQRCode(content string) ([]byte, error) {
	return qrcode.Encode(content, qrcode.Medium, 512)
}
