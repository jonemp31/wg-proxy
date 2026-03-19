package wireguard

import "fmt"

func GenerateClientConfig(privateKey, psk, wgIP, serverPubKey, endpoint, subnetCIDR string) string {
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s/32

[Peer]
PublicKey = %s
PresharedKey = %s
Endpoint = %s
AllowedIPs = %s
PersistentKeepalive = 25`, privateKey, wgIP, serverPubKey, psk, endpoint, subnetCIDR)
}
