package icmp

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/irai/packet"
	"github.com/irai/packet/fastlog"
	"inet.af/netaddr"

	"golang.org/x/net/ipv6"
)

// Debug turn on logging
var Debug bool

const module4 = "icmp4"
const module6 = "icmp6"

// Event represents and ICMP6 event from a host
type Event struct {
	Type ipv6.ICMPType
	Host packet.Host
}

type ICMP6Handler interface {
	FindRouter(net.IP) Router
	PingAll() error
	packet.PacketProcessor
}
type ICMP6NOOP struct{}

func (p ICMP6NOOP) Start() error   { return nil }
func (p ICMP6NOOP) Stop() error    { return nil }
func (p ICMP6NOOP) PingAll() error { return nil }
func (p ICMP6NOOP) ProcessPacket(*packet.Host, []byte, []byte) (packet.Result, error) {
	return packet.Result{}, nil
}
func (p ICMP6NOOP) StartHunt(addr packet.Addr) (packet.HuntStage, error) {
	return packet.StageNoChange, nil
}
func (p ICMP6NOOP) StopHunt(addr packet.Addr) (packet.HuntStage, error) {
	return packet.StageNoChange, nil
}
func (p ICMP6NOOP) CheckAddr(addr packet.Addr) (packet.HuntStage, error) {
	return packet.StageNoChange, nil
}
func (p ICMP6NOOP) Close() error                     { return nil }
func (p ICMP6NOOP) MinuteTicker(now time.Time) error { return nil }
func (p ICMP6NOOP) FindRouter(net.IP) Router         { return Router{} }

var _ ICMP6Handler = &Handler6{}
var _ ICMP6Handler = &ICMP6NOOP{}

// Handler implements ICMPv6 Neighbor Discovery Protocol
// see: https://mdlayher.com/blog/network-protocol-breakdown-ndp-and-go/
type Handler6 struct {
	Router     *Router
	LANRouters map[netaddr.IP]*Router
	session    *packet.Session
	huntList   packet.AddrList
	closed     bool
	closeChan  chan bool
	sync.Mutex
}

// PrintTable logs ICMP6 tables to standard out
func (h *Handler6) PrintTable() {
	table := h.session.GetHosts()
	if len(table) > 0 {
		fmt.Printf("icmp6 hosts table len=%v\n", len(table))
		for _, host := range table {
			host.MACEntry.Row.RLock()
			if packet.IsIP6(host.Addr.IP) {
				fmt.Printf("mac=%s ip=%v online=%v \n", host.MACEntry.MAC, host.Addr.IP, host.Online)
			}
			host.MACEntry.Row.RUnlock()
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

// New creates an ICMP6 handler and attach to the engine
func New6(session *packet.Session) (*Handler6, error) {

	h := &Handler6{LANRouters: make(map[netaddr.IP]*Router), closeChan: make(chan bool)}
	h.session = session

	return h, nil
}

// Close removes the plugin from the engine
func (h *Handler6) Close() error {
	h.closed = true
	close(h.closeChan)
	return nil
}

// Start prepares to accept packets
func (h *Handler6) Start() error {
	if err := h.SendRouterSolicitation(); err != nil {
		return err
	}
	if err := ExecPing(packet.IP6AllNodesMulticast.String() + "%" + h.session.NICInfo.IFI.Name); err != nil { // ping with external cmd tool
		fmt.Printf("icmp6 : error in initial ping all nodes multicast - ignoring : %s\n", err)
	}
	return nil
}

// Stop implements PacketProcessor interface
func (h *Handler6) Stop() error {
	h.Close()
	return nil
}

func (h *Handler6) PingAll() error {
	if h.session.NICInfo.HostLLA.IP == nil {
		return packet.ErrInvalidIP6LLA
	}
	fmt.Println("icmp6 : ping all")
	return h.SendEchoRequest(packet.Addr{MAC: h.session.NICInfo.HostMAC, IP: h.session.NICInfo.HostLLA.IP}, packet.IP6AllNodesAddr, 99, 1)
}

// MinuteTicker implements packet processor interface
// Send echo request to all nodes
func (h *Handler6) MinuteTicker(now time.Time) error {
	return h.SendEchoRequest(packet.Addr{MAC: h.session.NICInfo.HostMAC, IP: h.session.NICInfo.HostLLA.IP}, packet.IP6AllNodesAddr, 199, 1)
}

// HuntStage implements PacketProcessor interface
func (h *Handler6) CheckAddr(addr packet.Addr) (packet.HuntStage, error) {
	if h.session.NICInfo.HostLLA.IP == nil { // in case host does not have IPv6
		return packet.StageNoChange, nil
	}
	srcAddr := packet.Addr{MAC: h.session.NICInfo.HostMAC, IP: h.session.NICInfo.HostLLA.IP}

	// Neigbour solicitation almost always result in a response from host if online unless
	// host is on battery saving mode
	if addr.IP.IsLinkLocalUnicast() {
		if err := h.SendNeighbourSolicitation(srcAddr, packet.IPv6SolicitedNode(addr.IP), addr.IP); err != nil {
			fmt.Printf("icmp6 : error checking address %s error=\"%s\"", addr, err)
		}
		return packet.StageNoChange, nil
	}

	// ping response is optional and could be disabled on a given host
	if err := h.Ping(srcAddr, addr, time.Second*2); err != nil {
		return packet.StageNoChange, packet.ErrTimeout
	}

	return packet.StageNormal, nil
}

func (h *Handler6) sendPacket(srcAddr packet.Addr, dstAddr packet.Addr, b []byte) error {
	buf := packet.EtherBufferPool.Get().(*[packet.EthMaxSize]byte)
	defer packet.EtherBufferPool.Put(buf)
	ether := packet.Ether(buf[:])

	// All Neighbor Discovery packets must use link-local addresses (FE80::/64)
	// and a hop limit of 255. Linux discards ND messages with hop limits different than 255.
	hopLimit := uint8(64)
	if dstAddr.IP.IsLinkLocalUnicast() || dstAddr.IP.IsLinkLocalMulticast() {
		hopLimit = 255
	}

	ether = packet.EtherMarshalBinary(ether, syscall.ETH_P_IPV6, h.session.NICInfo.HostMAC, dstAddr.MAC)
	ip6 := packet.IP6MarshalBinary(ether.Payload(), hopLimit, srcAddr.IP, dstAddr.IP)
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
	ICMP(ip6.Payload()).SetChecksum(packet.Checksum(psh))

	if _, err := h.session.Conn.WriteTo(ether, &dstAddr); err != nil {
		fmt.Println("icmp6 : failed to write ", err)
		return err
	}

	return nil
}

var repeat int = -1

// ProcessPacket handles icmp6 packets
func (h *Handler6) ProcessPacket(host *packet.Host, p []byte, header []byte) (result packet.Result, err error) {

	ether := packet.Ether(p)
	ip6Frame := packet.IP6(ether.Payload())
	icmp6Frame := ICMP(header)

	if !icmp6Frame.IsValid() {
		fastlog.NewLine(module6, "error invalid icmp frame").Struct(ether).Int("len", len(header)).ByteArray("frame", header).Write()
		return packet.Result{}, errParseMessage
	}

	t := ipv6.ICMPType(icmp6Frame.Type())
	if Debug && t != ipv6.ICMPTypeRouterAdvertisement {
		fastlog.NewLine("icmp6", "ether").Struct(ether).Module("icmp6", "ip6").Struct(ip6Frame).Module("icmp6", "icmp").Struct(icmp6Frame).Write()
	}

	switch t {
	case ipv6.ICMPTypeNeighborAdvertisement: // 0x88
		frame := ICMP6NeighborAdvertisement(icmp6Frame)
		if !frame.IsValid() {
			fmt.Println("icmp6 : invalid NS msg")
			return packet.Result{}, packet.ErrParseFrame
		}
		if Debug {
			fastlog.NewLine("icmp6", "neighbor advertisement").IP("ip", ip6Frame.Src()).Struct(frame).Write()
			// fastlog.Strings("icmp6 : neighbor advertisement from ip=", ip6Frame.Src().String(), " ", frame.String())
		}

		// When a device gets an IPv6 address, it will join a solicited-node multicast group
		// to see if any other devices are trying to communicate with it. In this case, the
		// source IP is sometimes ff02::1 multicast, which means the host is nil.
		// If unsolicited and Override, it is an indication the IPv6 that corresponds to a link layer address has changed.
		if frame.Override() && !frame.Solicited() {
			fastlog.NewLine(module6, "neighbor advertisement overrid IP").Struct(ip6Frame).Module(module6, "neighbour advertisement").Struct(frame).Write()
			if frame.TargetLLA() == nil {
				fastlog.NewLine(module6, "error na override with nil targetLLA").Error(packet.ErrInvalidMAC).Write()
				return packet.Result{}, packet.ErrInvalidMAC
			}
			result.Update = true
			result.FrameAddr = packet.Addr{MAC: frame.TargetLLA(), IP: frame.TargetAddress()} // ok to pass frame addr
		}
		return result, nil

	case ipv6.ICMPTypeNeighborSolicitation: // 0x87
		frame := ICMP6NeighborSolicitation(icmp6Frame)
		if !frame.IsValid() {
			fmt.Println("icmp6 : invalid NS msg")
			return packet.Result{}, packet.ErrParseFrame
		}
		if Debug {
			fastlog.NewLine("icmp6", "neighbor solicitation").IP("ip", ip6Frame.Src()).Struct(frame).Write()
		}

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
			if Debug {
				fmt.Printf("icmp6 : dad probe for target=%s srcip=%s srcmac=%s dstip=%s dstmac=%s\n", frame.TargetAddress(), ip6Frame.Src(), ether.Src(), ip6Frame.Dst(), ether.Dst())
			}
			result.Update = true
			result.FrameAddr = packet.Addr{MAC: ether.Src(), IP: frame.TargetAddress()} // ok to pass frame addr
			return result, nil
		}

		// If a host is looking up for a GUA on the lan, it is likely a valid IP6 GUA for a local host.
		// So, send our own neighbour solicitation to discover the IP
		if frame.TargetAddress().IsGlobalUnicast() {
			srcAddr := packet.Addr{MAC: h.session.NICInfo.HostMAC, IP: h.session.NICInfo.HostLLA.IP}
			dstAddr := packet.Addr{MAC: ether.Dst(), IP: ip6Frame.Dst()}
			h.SendNeighbourSolicitation(srcAddr, dstAddr, frame.TargetAddress())
		}
		return packet.Result{}, nil

	case ipv6.ICMPTypeRouterAdvertisement: // 0x86
		frame := ICMP6RouterAdvertisement(icmp6Frame)
		if !frame.IsValid() {
			fmt.Println("icmp6 : invalid icmp6 ra msg")
			return packet.Result{}, packet.ErrParseFrame
		}

		// wakeup all pending spoof goroutines
		// we want to immediately spoof hosts after a RA
		if h.huntList.Len() > 0 {
			ch := h.closeChan
			h.closeChan = make(chan bool)
			close(ch) // this will cause all spoof loop select to wakeup
		}

		repeat++
		if repeat%4 != 0 { // skip if too often - home router send RA every 4 sec
			break
		}

		// Protect agains nil host
		// NS source IP is sometimes ff02::1 (multicast), which means that host is not in the table (nil)
		if host == nil {
			return packet.Result{}, fmt.Errorf("ra host cannot be nil")
		}
		options, err := frame.Options()
		if err != nil {
			fmt.Printf("icmp6 : invalid options %s\n", err)
			return packet.Result{}, err
		}

		mac := options.SourceLLA.MAC
		if mac == nil || len(mac) != ethAddrLen {
			mac = ether.Src()
			fmt.Printf("icmp6 : options missing sourceLLA options=%v\n", options)
		}

		h.Lock()
		router, found := h.findOrCreateRouter(mac, ip6Frame.Src())
		router.ManagedFlag = frame.ManagedConfiguration()
		router.OtherCondigFlag = frame.OtherConfiguration()
		router.Preference = frame.Preference()
		router.CurHopLimit = frame.CurrentHopLimit()
		router.DefaultLifetime = time.Duration(time.Duration(frame.Lifetime()) * time.Second)
		router.ReacheableTime = int(frame.ReachableTime())
		router.RetransTimer = int(frame.RetransmitTimer())
		curPrefix := router.Options.FirstPrefix // keep current prefix
		router.Options = options
		router.Prefixes = options.Prefixes
		h.Unlock()

		if Debug {
			l := fastlog.NewLine("icmp6", "ether").Struct(ether).Module("icmp6", "ip6").Struct(ip6Frame)
			l.Module("icmp6", "router advertisement").Struct(icmp6Frame).Sprintf("options", router.Options)
			l.Write()
		}

		result := packet.Result{}
		//notify if first time or if prefix changed
		if !found || !curPrefix.Equal(router.Options.FirstPrefix) {
			result = packet.Result{Update: true, IsRouter: true}
		}
		return result, nil

	case ipv6.ICMPTypeRouterSolicitation:
		frame := ICMP6RouterSolicitation(icmp6Frame)
		if err := frame.IsValid(); err != nil {
			return packet.Result{}, err
		}
		if Debug {
			fastlog.NewLine("icmp6", "router solicitation").IP("ip", ip6Frame.Src()).Struct(frame).Write()
		}

		// Source address:
		//    - usually the unspecified IPv6 address (0:0:0:0:0:0:0:0) or
		//      configured unicast address of the interface.
		// Destination address:
		//    - the all-routers multicast address (FF02::2) with the link-local scope.
		return packet.Result{}, nil
	case ipv6.ICMPTypeEchoReply: // 0x81
		echo := ICMPEcho(icmp6Frame)
		if !echo.IsValid() {
			return packet.Result{}, fmt.Errorf("invalid icmp echo msg len=%d", len(icmp6Frame))
		}
		if Debug {
			fmt.Printf("icmp6 : echo reply from ip=%s %s\n", ip6Frame.Src(), echo)
		}
		echoNotify(echo.EchoID()) // unblock ping if waiting
		return packet.Result{}, nil

	case ipv6.ICMPTypeEchoRequest: // 0x80
		echo := ICMPEcho(icmp6Frame)
		if Debug {
			// fmt.Printf("icmp6 : echo request from ip=%s %s\n", ip6Frame.Src(), echo)
			fastlog.NewLine(module6, "echo recvd").IP("srcIP", ip6Frame.Src()).IP("dstIP", ip6Frame.Dst()).Struct(echo).Write()
		}
		return packet.Result{}, nil

	case ipv6.ICMPTypeMulticastListenerReport:
		fastlog.NewLine(module6, "multicast listener report recv").IP("ip", ip6Frame.Src()).Write()
		return packet.Result{}, nil

	case ipv6.ICMPTypeVersion2MulticastListenerReport:
		fastlog.NewLine(module6, "multicast listener report V2 recv").IP("ip", ip6Frame.Src()).Write()
		return packet.Result{}, nil

	case ipv6.ICMPTypeMulticastListenerQuery:
		fastlog.NewLine(module6, "multicast listener query recv").IP("ip", ip6Frame.Src()).Write()
		return packet.Result{}, nil

	case ipv6.ICMPTypeRedirect:
		redirect := ICMP6Redirect(icmp6Frame)
		if !redirect.IsValid() {
			return packet.Result{}, fmt.Errorf("invalid icmp redirect msg len=%d", len(redirect))
		}
		// fmt.Printf("icmp6 : redirect from ip=%s %s \n", ip6Frame.Src(), redirect)
		fastlog.NewLine(module6, "redirect recv").IP("fromIP", ip6Frame.Src()).Stringer(redirect).Write()

		return packet.Result{}, nil

	case ipv6.ICMPTypeDestinationUnreachable:
		if Debug {
			fastlog.NewLine(module6, "destination unreachable").Struct(ip6Frame).Struct(icmp6Frame).Write()
		}
		return packet.Result{}, nil

	default:
		fmt.Printf("icmp6 : type not implemented from ip=%s type=%v\n", ip6Frame.Src(), t)
		return packet.Result{}, fmt.Errorf("unrecognized icmp6 type=%d: %w", t, errParseMessage)
	}

	return packet.Result{}, nil
}
