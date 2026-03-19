package wireguard

import (
	"fmt"
	"os/exec"
	"strings"
)

func GenerateKeyPair() (privateKey, publicKey string, err error) {
	privOut, err := exec.Command("wg", "genkey").Output()
	if err != nil {
		return "", "", fmt.Errorf("wg genkey: %w", err)
	}
	privateKey = strings.TrimSpace(string(privOut))

	cmd := exec.Command("wg", "pubkey")
	cmd.Stdin = strings.NewReader(privateKey)
	pubOut, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("wg pubkey: %w", err)
	}
	publicKey = strings.TrimSpace(string(pubOut))

	return privateKey, publicKey, nil
}

func GeneratePSK() (string, error) {
	out, err := exec.Command("wg", "genpsk").Output()
	if err != nil {
		return "", fmt.Errorf("wg genpsk: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
