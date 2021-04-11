package icmp6

import (
	"fmt"
	"net"
	"time"

	"github.com/irai/packet"
)

// Cloudflare DNS IP6: 2606:4700:4700::1111 or 2606:4700:4700::1001
var (
	DNS6Cloudflare1 = net.IP{0x26, 0x06, 0x47, 0x00, 0x47, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0x11, 0x11}
	DNS6Cloudflare2 = net.IP{0x26, 0x06, 0x47, 0x00, 0x47, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0x10, 0x01}
	RDNSSCLoudflare = &RecursiveDNSServer{
		Lifetime: time.Minute * 10,
		Servers:  []net.IP{DNS6Cloudflare1, DNS6Cloudflare2},
	}
)

// My home IP6 prefix: Internode
// useful for testing
var (
	// home: 2001:4479:1901:a001
	prefix = net.IP{0x20, 0x01, 0x44, 0x79, 0x19, 0x01, 0xa0, 0x01, 0, 0, 0, 0, 0, 0, 0, 0}

	MyHomePrefix = []PrefixInformation{
		{
			PrefixLength:                   64,
			Prefix:                         prefix,
			AutonomousAddressConfiguration: true,
			ValidLifetime:                  time.Minute * 10,
			PreferredLifetime:              time.Minute * 5,
		},
	}
)

// Router holds a router identification
type Router struct {
	Addr            packet.Addr
	enableRADVS     bool // if true, we respond for this server
	ManagedFlag     bool // if true, hosts should get IP from DHCP, if false, use SLAAC IP
	OtherCondigFlag bool // if true, hosts should get other info from DHCP
	Preference      byte
	MTU             uint32
	ReacheableTime  int // Must be no greater than 3,600,000 milliseconds (1hour)
	RetransTimer    int //
	CurHopLimit     uint8
	DefaultLifetime time.Duration // A value of zero means the router is not to be used as a default router
	Prefixes        []PrefixInformation
	RDNSS           *RecursiveDNSServer // Pointer to facilitate comparison
	Options         NewOptions
}

func (h *Handler) findOrCreateRouter(mac net.HardwareAddr, ip net.IP) (router *Router, found bool) {
	r, found := h.LANRouters[string(ip)]
	if found {
		return r, true
	}
	router = &Router{Addr: packet.Addr{MAC: packet.CopyMAC(mac), IP: packet.CopyIP(ip)}}
	h.LANRouters[string(ip)] = router
	return router, false
}

type RADVS struct {
	h           *Handler
	Router      *Router
	stopChannel chan bool
}

func (h *Handler) StartRADVS(managed bool, other bool, prefixes []PrefixInformation, rdnss *RecursiveDNSServer) (*RADVS, error) {
	return h.startRADVS(managed, other, prefixes, rdnss)
}

func (h *Handler) startRADVS(managed bool, other bool, prefixes []PrefixInformation, rdnss *RecursiveDNSServer) (radvs *RADVS, err error) {
	radvs = &RADVS{stopChannel: make(chan bool, 1)}
	radvs.Router, _ = h.findOrCreateRouter(h.engine.NICInfo.HostMAC, h.engine.NICInfo.HostLLA.IP)
	radvs.Router.enableRADVS = true
	radvs.Router.ManagedFlag = managed
	radvs.Router.OtherCondigFlag = other
	radvs.Router.MTU = uint32(h.engine.NICInfo.IFI.MTU)
	radvs.Router.ReacheableTime = int((time.Minute * 10).Milliseconds()) // Must be no greater than 3,600,000 milliseconds (1hour)
	radvs.Router.RetransTimer = int((time.Minute * 2).Milliseconds())
	radvs.Router.CurHopLimit = 1
	radvs.Router.DefaultLifetime = time.Minute * 30 // A value of zero means the router is not to be used as a default router
	radvs.Router.Prefixes = prefixes
	radvs.Router.RDNSS = rdnss
	radvs.h = h

	go radvs.sendAdvertistementLoop()

	return radvs, nil
}

func (r *RADVS) Stop() {
	r.stopChannel <- true
}

func (r *RADVS) SendRA() error {
	return r.h.SendRouterAdvertisement(*r.Router, packet.IP6AllNodesAddr)
}

func (r *RADVS) sendAdvertistementLoop() {
	r.h.SendRouterAdvertisement(*r.Router, packet.IP6AllNodesAddr)
	ticker := time.NewTicker(time.Duration(int64(time.Millisecond) * int64(r.Router.RetransTimer))).C
	for {
		select {
		case <-r.stopChannel:
			return

		case <-ticker:
			if err := r.h.SendRouterAdvertisement(*r.Router, packet.IP6AllNodesAddr); err != nil {
				fmt.Printf("icmp6: error in send ra: %s", err)
			}
		}
	}
}
