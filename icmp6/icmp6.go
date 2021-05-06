package icmp6

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/irai/packet"
	"github.com/irai/packet/model"
	log "github.com/sirupsen/logrus"
	"inet.af/netaddr"

	"golang.org/x/net/ipv6"
)

// Debug packets turn on logging if desirable
var Debug bool

// Event represents and ICMP6 event from a host
type Event struct {
	Type ipv6.ICMPType
	Host model.Host
}

var _ model.PacketProcessor = &ICMP6Handler{}

// ICMP6Handler implements ICMPv6 Neighbor Discovery Protocol
// see: https://mdlayher.com/blog/network-protocol-breakdown-ndp-and-go/
type ICMP6Handler struct {
	Router     *Router
	LANRouters map[netaddr.IP]*Router
	session    *model.Session
	huntList   model.AddrList
	closed     bool
	closeChan  chan bool
	sync.Mutex
}

// PrintTable logs ICMP6 tables to standard out
func (h *ICMP6Handler) PrintTable() {
	table := h.session.GetHosts()
	if len(table) > 0 {
		fmt.Printf("icmp6 hosts table len=%v\n", len(table))
		for _, host := range table {
			host.Row.RLock()
			if model.IsIP6(host.IP) {
				fmt.Printf("mac=%s ip=%v online=%v \n", host.MACEntry.MAC, host.IP, host.Online)
			}
			host.Row.RUnlock()
		}
	}

	if len(h.LANRouters) > 0 {
		fmt.Printf("icmp6 routers table len=%v\n", len(h.LANRouters))
		for _, v := range h.LANRouters {
			flags := ""
			if v.ManagedFlag {
				flags = flags + "M"
			}
			if v.OtherCondigFlag {
				flags = flags + "O"
			}
			fmt.Printf("%s flags=%s prefixes=%v rdnss=%+v options=%+v\n", v.Addr, flags, v.Prefixes, v.RDNSS, v.Options)
		}
	}
}

// Config define server configuration values
type Config struct {
	GlobalUnicastAddress net.IPNet
	LocalLinkAddress     net.IPNet
	UniqueLocalAddress   net.IPNet
}

// Attach creates an ICMP6 handler and attach to the engine
func Attach(session *model.Session) (*ICMP6Handler, error) {

	h := &ICMP6Handler{LANRouters: make(map[netaddr.IP]*Router), closeChan: make(chan bool)}
	h.session = session
	// engine.HandlerICMP6 = h

	return h, nil
}

// Detach removes the plugin from the engine
func (h *ICMP6Handler) Detach() error {
	h.closed = true
	close(h.closeChan)
	// h.engine.HandlerICMP6 = model.PacketNOOP{}
	return nil
}

// Start prepares to accept packets
func (h *ICMP6Handler) Start() error {
	if err := h.SendRouterSolicitation(); err != nil {
		return err
	}
	return packet.Ping(model.IP6AllNodesMulticast) // ping with external cmd tool
	// return h.SendEchoRequest(model.Addr{MAC: h.engine.NICInfo.HostMAC, IP: h.engine.NICInfo.HostLLA.IP}, model.IP6AllNodesAddr, 0, 0)
}

// Stop implements PacketProcessor interface
func (h *ICMP6Handler) Stop() error {
	return nil
}

// MinuteTicker implements packet processor interface
func (h *ICMP6Handler) MinuteTicker(now time.Time) error {
	return nil
}

// HuntStage implements PacketProcessor interface
func (h *ICMP6Handler) CheckAddr(addr model.Addr) (model.HuntStage, error) {
	if err := h.Ping(model.Addr{MAC: h.session.NICInfo.HostMAC, IP: h.session.NICInfo.HostLLA.IP}, addr, time.Second*2); err != nil {
		return model.StageNoChange, model.ErrTimeout
	}
	return model.StageNormal, nil
}

func (h *ICMP6Handler) sendPacket(srcAddr model.Addr, dstAddr model.Addr, b []byte) error {
	ether := model.Ether(make([]byte, model.EthMaxSize)) // Ping is called many times concurrently by client

	hopLimit := uint8(64)
	if dstAddr.IP.IsLinkLocalUnicast() || dstAddr.IP.IsLinkLocalMulticast() {
		hopLimit = 1
	}

	ether = model.EtherMarshalBinary(ether, syscall.ETH_P_IPV6, srcAddr.MAC, dstAddr.MAC)
	ip6 := model.IP6MarshalBinary(ether.Payload(), hopLimit, srcAddr.IP, dstAddr.IP)
	ip6, _ = ip6.AppendPayload(b, syscall.IPPROTO_ICMPV6)
	ether, _ = ether.SetPayload(ip6)

	// Calculate checksum of the pseudo header
	// The ICMPv6 checksum takes into account a pseudoheader of 40 bytes, which is a derivative of the real IPv6 header
	// which is composed as follows (in order):
	//   - 16 bytes for the source address
	//   - 16 bytes for the destination address
	//   - 4 bytes high endian payload length (the same value as in the IPv6 header)
	//   - 3 bytes zero
	//   - 1 byte nextheader (so, 58 decimal)
	psh := make([]byte, 40+len(b))
	copy(psh[0:16], ip6.Src())
	copy(psh[16:32], ip6.Dst())
	binary.BigEndian.PutUint32(psh[32:36], uint32(len(b)))
	psh[39] = 58
	copy(psh[40:], b)
	ICMP6(ip6.Payload()).SetChecksum(model.Checksum(psh))

	// icmp6 := ICMP6(model.IP6(ether.Payload()).Payload())
	// fmt.Println("DEBUG icmp :", icmp6, len(icmp6))
	// fmt.Println("DEBUG ether:", ether, len(ether), len(b))
	if _, err := h.session.Conn.WriteTo(ether, &model.Addr{MAC: dstAddr.MAC}); err != nil {
		log.Error("icmp failed to write ", err)
		return err
	}

	return nil
}

var repeat int = -1

// ProcessPacket handles icmp6 packets
func (h *ICMP6Handler) ProcessPacket(host *model.Host, p []byte, header []byte) (*model.Host, model.Result, error) {

	ether := model.Ether(p)
	ip6Frame := model.IP6(ether.Payload())
	icmp6Frame := ICMP6(header)

	if !icmp6Frame.IsValid() {
		return host, model.Result{}, fmt.Errorf("invalid icmp msg=%v: %w", icmp6Frame, errParseMessage)
	}

	t := ipv6.ICMPType(icmp6Frame.Type())
	if Debug && t != ipv6.ICMPTypeRouterAdvertisement {
		fmt.Println("ether:", ether)
		fmt.Println("ip6  :", ip6Frame)
		fmt.Println("icmp6:", icmp6Frame)
	}

	switch t {
	case ipv6.ICMPTypeNeighborAdvertisement: // 0x88
		frame := ICMP6NeighborAdvertisement(icmp6Frame)
		if !frame.IsValid() {
			fmt.Println("icmp6 : invalid NS msg")
			return host, model.Result{}, model.ErrParseMessage
		}
		fmt.Printf("icmp6 : neighbor advertisement from ip=%s %s\n", ip6Frame.Src(), frame)

		// Source IP is sometimes ff02::1 multicast, which means the host is nil
		if host == nil {
			host, _ = h.session.FindOrCreateHost(ether.Src(), frame.TargetAddress()) // will lock/unlock mutex
		}

	case ipv6.ICMPTypeNeighborSolicitation: // 0x87
		frame := ICMP6NeighborSolicitation(icmp6Frame)
		if !frame.IsValid() {
			fmt.Println("icmp6 : invalid NS msg")
			return host, model.Result{}, model.ErrParseMessage
		}
		fmt.Printf("icmp6 : neighbor solicitation from ip=%s %s\n", ip6Frame.Src(), frame)

		// Source address:
		//   - Either an address assigned to the interface from which this message was sent or
		//     the unspecified address (if duplicated address detection in progress).
		// Destination address:
		//   - Either the solicited-node multicast address (ff02::1..) corresponding to the target address, or
		//     the target address.
		//
		//IPv6 Duplicate Address Detection
		// IP6 src=0x00 dst=solicited-node address (multicast)
		//
		if ip6Frame.Src().IsUnspecified() {
			fmt.Printf("icmp6 : dad probe for target=%s srcip=%s srcmac=%s dstip=%s dstmac=%s\n", frame.TargetAddress(), ip6Frame.Src(), ether.Src(), ip6Frame.Dst(), ether.Dst())
			host, _ = h.session.FindOrCreateHost(ether.Src(), frame.TargetAddress()) // will lock/unlock mutex
		}

	case ipv6.ICMPTypeRouterAdvertisement: // 0x86

		frame := ICMP6RouterAdvertisement(icmp6Frame)
		if !frame.IsValid() {
			fmt.Println("icmp6 : invalid icmp6 ra msg")
			return host, model.Result{}, model.ErrParseMessage
		}

		repeat++
		if repeat%4 != 0 { // skip if too often - home router send RA every 4 sec
			break
		}

		// Protect agains nil host
		// NS source IP is sometimes ff02::1 (multicast), which means that host is not in the table (nil)
		if host == nil {
			return host, model.Result{}, fmt.Errorf("ra host cannot be nil")
		}
		options, err := frame.Options()
		if err != nil {
			fmt.Printf("icmp6 : invalid options %s\n", err)
			return host, model.Result{}, err
		}

		//
		h.Lock()
		router, _ := h.findOrCreateRouter(options.SourceLLA.Addr, ip6Frame.Src())
		router.ManagedFlag = frame.ManagedConfiguration()
		router.OtherCondigFlag = frame.OtherConfiguration()
		router.Preference = frame.Preference()
		router.CurHopLimit = frame.CurrentHopLimit()
		router.DefaultLifetime = time.Duration(time.Duration(frame.Lifetime()) * time.Second)
		router.ReacheableTime = int(frame.ReachableTime())
		router.RetransTimer = int(frame.RetransmitTimer())
		router.Options = options
		h.Unlock()

		if Debug {
			fmt.Println("ether :", ether)
			fmt.Println("ip6   :", ip6Frame)
		}
		fmt.Printf("icmp6 : router advertisement from ip=%s %s %+v \n", ip6Frame.Src(), frame, router.Options)

	case ipv6.ICMPTypeRouterSolicitation:
		frame := ICMP6RouterSolicitation(icmp6Frame)
		if !frame.IsValid() {
			return host, model.Result{}, model.ErrParseMessage
		}
		if Debug {
			fmt.Printf("icmp6 : router solicitation from ip=%s %s\n", ip6Frame.Src(), frame)
		}

		// Source address:
		//    - usually the unspecified IPv6 address (0:0:0:0:0:0:0:0) or
		//      configured unicast address of the interface.
		// Destination address:
		//    - the all-routers multicast address (FF02::2) with the link-local scope.

		/**
		for _, v := range h.LANRouters {
			if v.enableRADVS {
				if bytes.Equal(ether.Src(), msg.SourceLLA) {
					fmt.Printf("icmp6 error: source link address differ: ether=%s rs=%s\n", ether.Src(), ip6Frame.Src())
				}
				h.SendRouterAdvertisement(v, model.Addr{MAC: ether.Src(), IP: ip6Frame.Src()})
			}
		}
		**/

	case ipv6.ICMPTypeEchoReply: // 0x81
		echo := model.ICMPEcho(icmp6Frame)
		if !echo.IsValid() {
			return host, model.Result{}, fmt.Errorf("invalid icmp echo msg len=%d", len(icmp6Frame))
		}
		if Debug {
			fmt.Printf("icmp6 : echo reply from ip=%s %s\n", ip6Frame.Src(), echo)
		}
		echoNotify(echo.EchoID()) // unblock ping if waiting

	case ipv6.ICMPTypeEchoRequest: // 0x80
		echo := model.ICMPEcho(icmp6Frame)
		if Debug {
			fmt.Printf("icmp6 : echo request from ip=%s %s\n", ip6Frame.Src(), echo)
		}

	case ipv6.ICMPTypeMulticastListenerReport:
		fmt.Printf("icmp6 : multicast listener report from ip=%s \n", ip6Frame.Src())

	case ipv6.ICMPTypeVersion2MulticastListenerReport:
		fmt.Printf("icmp6 : multicast listener report V2 from ip=%s \n", ip6Frame.Src())

	case ipv6.ICMPTypeMulticastListenerQuery:
		fmt.Printf("icmp6 : multicast listener query from ip=%s \n", ip6Frame.Src())

	default:
		log.Printf("icmp6 : type not implemented from ip=%s type=%v\n", ip6Frame.Src(), t)
		return host, model.Result{}, fmt.Errorf("unrecognized icmp6 type=%d: %w", t, errParseMessage)
	}

	return host, model.Result{}, nil
}
