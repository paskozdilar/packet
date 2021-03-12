// package test provides common testing functionality across the plugins.
//
// It enables full engine testing by sending any packet type.
package test

import (
	"context"
	"fmt"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/irai/packet"
	"github.com/irai/packet/arp"
	"github.com/irai/packet/dhcp4"
)

var (
	zeroMAC = net.HardwareAddr{0, 0, 0, 0, 0, 0}
	ip1     = net.IPv4(192, 168, 0, 1)
	ip2     = net.IPv4(192, 168, 0, 2)
	ip3     = net.IPv4(192, 168, 0, 3)
	ip4     = net.IPv4(192, 168, 0, 4)
	ip5     = net.IPv4(192, 168, 0, 5)

	hostMAC   = net.HardwareAddr{0x00, 0x55, 0x55, 0x55, 0x55, 0x55}
	hostIP4   = net.IPv4(192, 168, 0, 129).To4()
	routerMAC = net.HardwareAddr{0x00, 0x66, 0x66, 0x66, 0x66, 0x66}
	routerIP4 = net.IPv4(192, 168, 0, 11).To4()
	homeLAN   = net.IPNet{IP: net.IPv4(192, 168, 0, 0), Mask: net.IPv4Mask(255, 255, 255, 0)}

	mac1 = net.HardwareAddr{0x00, 0x02, 0x03, 0x04, 0x05, 0x01}
	mac2 = net.HardwareAddr{0x00, 0x02, 0x03, 0x04, 0x05, 0x02}
	mac3 = net.HardwareAddr{0x00, 0x02, 0x03, 0x04, 0x05, 0x03}
	mac4 = net.HardwareAddr{0x00, 0x02, 0x03, 0x04, 0x05, 0x04}
	mac5 = net.HardwareAddr{0x00, 0x02, 0x03, 0x04, 0x05, 0x05}

	ip6LLARouter = net.IP{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}
	ip6LLAHost   = net.IP{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x10, 0x10}
	ip6LLA1      = net.IP{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}
	ip6LLA2      = net.IP{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x02}
	ip6LLA3      = net.IP{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x03}
	ip6LLA4      = net.IP{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x04}
	ip6LLA5      = net.IP{0xfe, 0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x05}

	hostAddr   = packet.Addr{MAC: hostMAC, IP: hostIP4}
	routerAddr = packet.Addr{MAC: routerMAC, IP: routerIP4}

	dnsIP4 = net.IPv4(8, 8, 8, 8)
)

type testContext struct {
	inConn        net.PacketConn
	outConn       net.PacketConn
	clientInConn  net.PacketConn
	clientOutConn net.PacketConn
	packet        *packet.Handler
	arpHandler    *arp.Handler
	dhcp4Handler  *dhcp4.Handler
	dhcp4XID      uint16
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	responseTable [][]byte
	savedIP       net.IP // save the returned IP for use by subsequent calls
}

func readResponse(ctx context.Context, tc *testContext) error {
	buf := make([]byte, 2000)
	for {
		n, _, err := tc.outConn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != context.Canceled {
				panic(err)
			}
		}
		if ctx.Err() == context.Canceled {
			return nil
		}

		buf = buf[:n]
		ether := packet.Ether(buf)
		if !ether.IsValid() {
			s := fmt.Sprintf("error ether client packet %s", ether)
			panic(s)
		}

		// used for debuging - disable to avoid verbose logging
		if false {
			fmt.Printf("raw: got buffere msg=%s\n", ether)
		}
		tmp := make([]byte, len(buf))
		copy(tmp, buf)
		tc.responseTable = append(tc.responseTable, tmp)
	}
}

func setupTestHandler() *testContext {

	var err error

	tc := testContext{}
	tc.ctx, tc.cancel = context.WithCancel(context.Background())

	tc.inConn, tc.outConn = packet.TestNewBufferedConn()
	go readResponse(tc.ctx, &tc) // MUST read the out conn to avoid blocking the sender

	tc.clientInConn, tc.clientOutConn = packet.TestNewBufferedConn()
	go packet.TestReadAndDiscardLoop(tc.ctx, tc.clientOutConn) // must read to avoid blocking

	nicInfo := packet.NICInfo{
		HostMAC:   hostMAC,
		HostIP4:   net.IPNet{IP: hostIP4, Mask: net.IPv4Mask(255, 255, 255, 0)},
		RouterIP4: net.IPNet{IP: routerIP4, Mask: net.IPv4Mask(255, 255, 255, 0)},
		HomeLAN4:  homeLAN,
	}

	// override handler with conn and nicInfo
	config := packet.Config{Conn: tc.inConn, NICInfo: &nicInfo, ProbeInterval: time.Millisecond * 500, OfflineDeadline: time.Millisecond * 500, PurgeDeadline: time.Second * 2}
	tc.packet, err = config.NewEngine("eth0")
	if err != nil {
		panic(err)
	}
	if packet.Debug {
		fmt.Println("nicinfo: ", tc.packet.NICInfo)
	}

	tc.arpHandler, err = arp.Attach(tc.packet)
	if err != nil {
		panic(err)
	}

	// Default dhcp engine
	netfilterIP, err := packet.SegmentLAN("eth0",
		net.IPNet{IP: hostIP4, Mask: net.IPv4Mask(255, 255, 255, 0)},
		net.IPNet{IP: routerIP4, Mask: net.IPv4Mask(255, 255, 255, 0)})
	if err != nil {
		panic(err)
	}
	tc.dhcp4Handler, err = dhcp4.Config{ClientConn: tc.clientInConn}.Attach(tc.packet, net.IPNet{IP: netfilterIP.IP, Mask: net.IPv4Mask(255, 255, 255, 0)}, dnsIP4, "")
	if err != nil {
		panic("cannot create handler" + err.Error())
	}
	tc.dhcp4Handler.SetMode(dhcp4.ModeSecondaryServerNice)

	go func() {
		if err := tc.packet.ListenAndServe(tc.ctx); err != nil {
			panic(err)
		}
	}()

	time.Sleep(time.Millisecond * 10) // time for all goroutine to start
	return &tc
}

func (tc *testContext) Close() {
	time.Sleep(time.Millisecond * 20) // wait for all packets to finish
	if packet.Debug {
		fmt.Println("teminating context")
	}
	tc.cancel()
	tc.wg.Wait()
}

type testEvent struct {
	name             string
	action           string // capture, block, accept, release, event
	packetEvent      packet.Notification
	waitTimeAfter    time.Duration
	wantCapture      bool
	wantStage        packet.HuntStage
	wantOnline       bool
	hostTableInc     int // expected increment
	macTableInc      int // expected increment
	responseTableInc int // expected increment
	responsePos      int // position of response in responseTable -1 is the last entry
	srcAddr          packet.Addr
	dstAddr          packet.Addr
	ether            packet.Ether
	wantHost         *packet.Host
}

func newDHCP4DiscoverFrame(src packet.Addr, xid []byte) packet.Ether {
	options := []dhcp4.Option{}
	oDNS := dhcp4.Option{Code: dhcp4.OptionDomainNameServer, Value: []byte{}}

	var err error
	ether := packet.Ether(make([]byte, packet.EthMaxSize))
	ether = packet.EtherMarshalBinary(ether, syscall.ETH_P_IP, src.MAC, arp.EthernetBroadcast)
	ip4 := packet.IP4MarshalBinary(ether.Payload(), 50, src.IP, net.IPv4zero)
	udp := packet.UDPMarshalBinary(ip4.Payload(), packet.DHCP4ClientPort, packet.DHCP4ServerPort)
	dhcp4Frame := dhcp4.RequestPacket(dhcp4.Discover, src.MAC, src.IP, xid, false, append(options, oDNS))
	udp, err = udp.AppendPayload(dhcp4Frame)
	ip4 = ip4.SetPayload(udp, syscall.IPPROTO_UDP)
	if ether, err = ether.SetPayload(ip4); err != nil {
		panic(err.Error())
	}
	return ether
}

func newDHCP4RequestFrame(src packet.Addr, serverID net.IP, requestedIP net.IP, xid []byte) packet.Ether {
	options := []dhcp4.Option{}
	oDNS := dhcp4.Option{Code: dhcp4.OptionDomainNameServer, Value: []byte{}}
	oReqIP := dhcp4.Option{Code: dhcp4.OptionRequestedIPAddress, Value: requestedIP}
	oServerID := dhcp4.Option{Code: dhcp4.OptionServerIdentifier, Value: serverID}
	options = append(options, oDNS)
	options = append(options, oReqIP)
	options = append(options, oServerID)

	var err error
	ether := packet.Ether(make([]byte, packet.EthMaxSize))
	ether = packet.EtherMarshalBinary(ether, syscall.ETH_P_IP, src.MAC, arp.EthernetBroadcast)
	ip4 := packet.IP4MarshalBinary(ether.Payload(), 50, src.IP, net.IPv4zero)
	udp := packet.UDPMarshalBinary(ip4.Payload(), packet.DHCP4ClientPort, packet.DHCP4ServerPort)
	dhcp4Frame := dhcp4.RequestPacket(dhcp4.Request, src.MAC, requestedIP, xid, false, options)
	udp, err = udp.AppendPayload(dhcp4Frame)
	ip4 = ip4.SetPayload(udp, syscall.IPPROTO_UDP)
	if ether, err = ether.SetPayload(ip4); err != nil {
		panic(err.Error())
	}
	return ether
}

func newARPFrame(src packet.Addr, dst packet.Addr, operation uint16) packet.Ether {
	var err error
	ether := packet.Ether(make([]byte, packet.EthMaxSize))
	ether = packet.EtherMarshalBinary(ether, syscall.ETH_P_ARP, src.MAC, dst.MAC)
	arpFrame, err := arp.ARPMarshalBinary(ether.Payload(), operation, src.MAC, src.IP, dst.MAC, dst.IP)
	if ether, err = ether.SetPayload(arpFrame); err != nil {
		panic(err.Error())
	}
	return ether
}

func newArpAnnoucementEvent(addr packet.Addr, hostInc int, macInc int) []testEvent {
	return []testEvent{
		{name: "arp-announcement-" + addr.MAC.String(), action: "arpAnnouncement", hostTableInc: hostInc, macTableInc: macInc, responsePos: -1, responseTableInc: 0,
			srcAddr:       addr,
			wantHost:      &packet.Host{IP: addr.IP, Online: true},
			waitTimeAfter: time.Millisecond * 10,
		},
	}
}

func newHostEvents(addr packet.Addr, hostInc int, macInc int) []testEvent {
	return []testEvent{
		{name: "discover-" + addr.MAC.String(), action: "dhcp4Discover", hostTableInc: 0, macTableInc: macInc, responsePos: -1, responseTableInc: 1,
			srcAddr:       packet.Addr{MAC: addr.MAC, IP: net.IPv4zero},
			wantHost:      nil, // don't validate host
			waitTimeAfter: time.Millisecond * 10,
		},
		{name: "request-" + addr.MAC.String(), action: "dhcp4Request", hostTableInc: hostInc, macTableInc: 0, responsePos: -1, responseTableInc: 1,
			srcAddr:       packet.Addr{MAC: addr.MAC, IP: net.IPv4zero},
			wantHost:      &packet.Host{IP: nil, Online: true},
			waitTimeAfter: time.Millisecond * 20,
		},
		{name: "arp-probe-" + addr.MAC.String(), action: "arpProbe", hostTableInc: 0, macTableInc: 0, responsePos: -1, responseTableInc: 0,
			srcAddr:       packet.Addr{MAC: mac1, IP: net.IPv4zero},
			wantHost:      &packet.Host{IP: nil, Online: true},
			waitTimeAfter: time.Millisecond * 10,
		},
		{name: "arp-announcement-" + addr.MAC.String(), action: "arpAnnouncement", hostTableInc: 0, macTableInc: 0, responsePos: -1, responseTableInc: 0,
			srcAddr:       packet.Addr{MAC: addr.MAC, IP: nil}, // set IP to zero to use savedIP
			wantHost:      &packet.Host{IP: nil, Online: true},
			waitTimeAfter: time.Millisecond * 10,
		},
	}
}

var buf = make([]byte, packet.EthMaxSize)

func runAction(t *testing.T, tc *testContext, tt testEvent) {

	switch tt.action {
	case "capture":
	case "release":

	case "dhcp4Request":
		tt.ether = newDHCP4RequestFrame(tt.srcAddr, hostIP4, tc.savedIP, []byte(fmt.Sprintf("%d", tc.dhcp4XID)))

	case "dhcp4Discover":
		tc.dhcp4XID++
		tt.ether = newDHCP4DiscoverFrame(tt.srcAddr, []byte(fmt.Sprintf("%d", tc.dhcp4XID)))

	case "arpProbe":
		if tc.savedIP == nil {
			panic("invalid savedIP")
		}
		tt.ether = newARPFrame(tt.srcAddr, packet.Addr{MAC: arp.EthernetBroadcast, IP: tc.savedIP}, arp.OperationRequest)

	case "arpAnnouncement":
		if tt.srcAddr.IP == nil {
			if tc.savedIP == nil {
				panic("invalid savedIP")
			}
			tt.srcAddr.IP = tc.savedIP
		}
		tt.ether = newARPFrame(packet.Addr{MAC: tt.srcAddr.MAC, IP: tt.srcAddr.IP.To4()}, packet.Addr{MAC: arp.EthernetBroadcast, IP: tt.srcAddr.IP.To4()}, arp.OperationRequest)

	default:
		fmt.Println("invalid action")
		return
	}

	savedResponseTableCount := len(tc.responseTable)
	savedHostTableCount := len(tc.packet.LANHosts.Table)
	savedMACTableCount := len(tc.packet.MACTable.Table)

	if _, err := tc.outConn.WriteTo(tt.ether, &packet.Addr{MAC: tt.ether.Dst()}); err != nil {
		panic(err.Error())
	}
	time.Sleep(tt.waitTimeAfter)

	if n := len(tc.packet.LANHosts.Table) - savedHostTableCount; n != tt.hostTableInc {
		t.Errorf("%s: invalid host table len want=%v got=%v", tt.name, tt.hostTableInc, n)
		tc.packet.PrintTable()
	}
	if n := len(tc.packet.MACTable.Table) - savedMACTableCount; n != tt.macTableInc {
		t.Errorf("%s: invalid mac table len want=%v got=%v", tt.name, tt.macTableInc, n)
	}
	if n := len(tc.responseTable) - savedResponseTableCount; n != tt.responseTableInc {
		t.Errorf("%s: invalid mac reponse count len want=%v got=%v", tt.name, tt.responseTableInc, n)
	}

	var ip net.IP = tt.srcAddr.IP
	switch tt.action {
	case "dhcp4Request", "dhcp4Discover":
		buf := tc.responseTable[len(tc.responseTable)-1]
		dhcp4Frame := dhcp4.DHCP4(packet.UDP(packet.IP4(packet.Ether(buf).Payload()).Payload()).Payload())
		ip = dhcp4Frame.YIAddr().To4()
		if ip == nil {
			panic("ip is nil")
		}
		tc.savedIP = ip
	}

	if tt.wantHost != nil {
		ip := tt.wantHost.IP
		if ip == nil {
			ip = tc.savedIP
		}
		host := tc.packet.FindIP(ip)
		if host == nil {
			t.Errorf("%s: host not found in table ip=%s ", tt.name, ip)
			return
		}
		if host.Online != tt.wantHost.Online {
			t.Errorf("%s: host incorrect online status want=%v got=%v ", tt.name, tt.wantHost.Online, host.Online)
		}
	}

}
