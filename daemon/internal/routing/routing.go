package routing

// Router is a no-op in the chain-proxy architecture.
// Traffic is routed by gost chaining to the phone's SOCKS5 proxy
// through the WireGuard tunnel — no policy routing needed.
type Router struct{}

func NewRouter(wgInterface string) *Router {
	return &Router{}
}
