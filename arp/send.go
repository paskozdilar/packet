package arp

import (
	"bytes"
	"fmt"
	"net"
	"syscall"
	"time"

	"errors"
	"log"

	"github.com/irai/packet/raw"
)

var (
	// ErrNotFound is returned when MAC not found
	ErrNotFound = errors.New("not found")

	writeTimeout, _ = time.ParseDuration("100ms")
	scanTimeout, _  = time.ParseDuration("5s")

	// EthernetBroadcast defines the broadcast address
	EthernetBroadcast = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
)

// Request send ARP request from src to dst
// multiple goroutines can call request simultaneously.
//
// Request is almost always broadcast but unicast can be used to maintain ARP table;
// i.e. unicast polling check for stale ARP entries; useful to test online/offline state
//
// ARP: packet types
//      note that RFC 3927 specifies 00:00:00:00:00:00 for Request TargetMAC
// +============+===+===========+===========+============+============+===================+===========+
// | Type       | op| dstMAC    | srcMAC    | SenderMAC  | SenderIP   | TargetMAC         |  TargetIP |
// +============+===+===========+===========+============+============+===================+===========+
// | request    | 1 | broadcast | clientMAC | clientMAC  | clientIP   | ff:ff:ff:ff:ff:ff |  targetIP |
// | reply      | 2 | clientMAC | targetMAC | targetMAC  | targetIP   | clientMAC         |  clientIP |
// | gratuitous | 2 | broadcast | clientMAC | clientMAC  | clientIP   | ff:ff:ff:ff:ff:ff |  clientIP |
// | ACD probe  | 1 | broadcast | clientMAC | clientMAC  | 0x00       | 0x00              |  targetIP |
// | ACD announ | 1 | broadcast | clientMAC | clientMAC  | clientIP   | ff:ff:ff:ff:ff:ff |  clientIP |
// +============+===+===========+===========+============+============+===================+===========+
//
func (c *Handler) Request(srcHwAddr net.HardwareAddr, srcIP net.IP, dstHwAddr net.HardwareAddr, dstIP net.IP) error {
	if Debug {
		if srcIP.Equal(dstIP) {
			log.Printf("arp send announcement - I am ip=%s mac=%s", srcIP, srcHwAddr)
		} else {
			log.Printf("arp send request - who is ip=%s tell sip=%s smac=%s", dstIP, srcIP, srcHwAddr)
		}
	}

	return c.requestWithDstEthernet(EthernetBroadcast, srcHwAddr, srcIP, dstHwAddr, dstIP)
}

func (c *Handler) request(srcHwAddr net.HardwareAddr, srcIP net.IP, dstHwAddr net.HardwareAddr, dstIP net.IP) error {
	return c.requestWithDstEthernet(EthernetBroadcast, srcHwAddr, srcIP, dstHwAddr, dstIP)
}

// buffer is a lockable buffer to avoid allocation
var buffer = raw.EtherBuffer{}

func (c *Handler) requestWithDstEthernet(dstEther net.HardwareAddr, srcHwAddr net.HardwareAddr, srcIP net.IP, dstHwAddr net.HardwareAddr, dstIP net.IP) error {
	ether := buffer.Alloc()
	defer buffer.Free()

	ether = raw.EtherMarshalBinary(ether, syscall.ETH_P_ARP, srcHwAddr, dstEther)
	arp, err := ARPMarshalBinary(ether.Payload(), OperationRequest, srcHwAddr, srcIP, dstHwAddr, dstIP)
	if err != nil {
		return err
	}
	n := len(ether) + len(arp)

	// if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
	// return err
	// }

	if _, err := c.conn.WriteTo(ether[:n], &raw.Addr{MAC: dstEther}); err != nil {
		return err
	}
	return nil
}

// Reply send ARP reply from the src to the dst
//
// Call with dstHwAddr = ethernet.Broadcast to reply to all
func (c *Handler) Reply(dstEther net.HardwareAddr, srcHwAddr net.HardwareAddr, srcIP net.IP, dstHwAddr net.HardwareAddr, dstIP net.IP) error {
	if Debug {
		log.Printf("arp send reply - ip=%s is at mac=%s", srcIP, srcHwAddr)
	}
	return c.reply(dstEther, srcHwAddr, srcIP, dstHwAddr, dstIP)
}

// reply sends a ARP reply packet from src to dst.
//
// dstEther identifies the target for the Ethernet packet : i.e. use EthernetBroadcast for gratuitous ARP
func (c *Handler) reply(dstEther net.HardwareAddr, srcHwAddr net.HardwareAddr, srcIP net.IP, dstHwAddr net.HardwareAddr, dstIP net.IP) error {
	ether := buffer.Alloc()
	defer buffer.Free()

	ether = raw.EtherMarshalBinary(ether, syscall.ETH_P_ARP, srcHwAddr, dstEther)
	arp, err := ARPMarshalBinary(ether.Payload(), OperationReply, srcHwAddr, srcIP, dstHwAddr, dstIP)
	if err != nil {
		return err
	}
	n := len(ether) + len(arp)

	// if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
	// return err
	// }

	_, err = c.conn.WriteTo(ether[:n], &raw.Addr{MAC: dstEther})
	return err
}

// Probe will send an arp request broadcast on the local link.
//
// The term 'ARP Probe' is used to refer to an ARP Request packet, broadcast on the local link,
// with an all-zero 'sender IP address'. The 'sender hardware address' MUST contain the hardware address of the
// interface sending the packet. The 'sender IP address' field MUST be set to all zeroes,
// to avoid polluting ARP caches in other hosts on the same link in the case where the address turns out
// to be already in use by another host. The 'target IP address' field MUST be set to the address being probed.
// An ARP Probe conveys both a question ("Is anyone using this address?") and an
// implied statement ("This is the address I hope to use.").
func (c *Handler) Probe(ip net.IP) error {
	return c.Request(c.NICInfo.HostMAC, net.IPv4zero, EthernetBroadcast, ip)
}

// probeUnicast is used to validate the client is still online; same as ARP probe but unicast to target
func (c *Handler) probeUnicast(mac net.HardwareAddr, ip net.IP) error {
	return c.Request(c.NICInfo.HostMAC, net.IPv4zero, mac, ip)
}

// announce sends arp announcement packet
//
// Having probed to determine that a desired address may be used safely,
// a host implementing this specification MUST then announce that it
// is commencing to use this address by broadcasting ANNOUNCE_NUM ARP
// Announcements, spaced ANNOUNCE_INTERVAL seconds apart.  An ARP
// Announcement is identical to the ARP Probe described above, except
// that now the sender and target IP addresses are both set to the
// host's newly selected IPv4 address.  The purpose of these ARP
// Announcements is to make sure that other hosts on the link do not
// have stale ARP cache entries left over from some other host that may
// previously have been using the same address.  The host may begin
// legitimately using the IP address immediately after sending the first
// of the two ARP Announcements;
func (c *Handler) announce(dstEther net.HardwareAddr, mac net.HardwareAddr, ip net.IP, targetMac net.HardwareAddr, repeats int) (err error) {
	if Debug {
		if bytes.Equal(dstEther, EthernetBroadcast) {
			log.Printf("arp send announcement broadcast - I am ip=%s mac=%s", ip, mac)
		} else {
			log.Printf("arp send announcement unicast - I am ip=%s mac=%s to=%s", ip, mac, dstEther)
		}
	}

	err = c.requestWithDstEthernet(dstEther, mac, ip, targetMac, ip)

	go func() {
		for i := 1; i < repeats; i++ {
			time.Sleep(time.Millisecond * 500)
			c.requestWithDstEthernet(dstEther, mac, ip, targetMac, ip)
		}
	}()
	return err
}

// WhoIs will send a request packet to get the MAC address for the IP. Retry 3 times.
//
func (c *Handler) WhoIs(ip net.IP) (raw.Addr, error) {

	for i := 0; i < 3; i++ {
		if host := c.LANHosts.FindIP(ip); host != nil {
			return raw.Addr{IP: host.IP, MAC: host.MAC}, nil
		}
		if err := c.Request(c.NICInfo.HostMAC, c.NICInfo.HostIP4.IP, EthernetBroadcast, ip); err != nil {
			return raw.Addr{}, fmt.Errorf("arp WhoIs error: %w", err)
		}
		time.Sleep(time.Millisecond * 50)
	}

	if Debug {
		log.Printf("arp ip=%s whois not found", ip)
		c.LANHosts.PrintTable()
	}
	return raw.Addr{}, ErrNotFound
}
