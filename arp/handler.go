package arp

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"log"

	"github.com/irai/packet"
	"github.com/irai/packet/raw"
)

// Config holds configuration parameters
//
// Set FullNetworkScanInterval = 0 to avoid network scan
type Config struct {
	NIC                     string           `yaml:"-"`
	HostMAC                 net.HardwareAddr `yaml:"-"`
	HostIP                  net.IP           `yaml:"-"`
	RouterIP                net.IP           `yaml:"-"`
	HomeLAN                 net.IPNet        `yaml:"-"`
	FullNetworkScanInterval time.Duration    `yaml:"-"` // Set it to zero if no scan required
	ProbeInterval           time.Duration    `yaml:"-"` // how often to probe if IP is online
	OfflineDeadline         time.Duration    `yaml:"-"` // mark offline if more than OfflineInte
	PurgeDeadline           time.Duration    `yaml:"-"`
}

func (c Config) String() string {
	return fmt.Sprintf("hostmac=%s hostIP=%s routerIP=%s homeLAN=%s scan=%v probe=%s offline=%v purge=%v",
		c.HostMAC, c.HostIP, c.RouterIP, c.HomeLAN, c.FullNetworkScanInterval, c.ProbeInterval, c.OfflineDeadline, c.PurgeDeadline)
}

// Handler stores instance variables
type Handler struct {
	conn        net.PacketConn
	table       *arpTable
	config      Config
	routerEntry MACEntry // store the router mac address
	sync.RWMutex
	notification chan<- MACEntry // notification channel for state change
}

var (
	// Debug - set Debug to true to see debugging messages
	Debug bool
)

// New creates an ARP handler for a given interface.
func New(config Config) (c *Handler, err error) {
	c = newHandler(config)

	ifi, err := net.InterfaceByName(config.NIC)
	if err != nil {
		return nil, fmt.Errorf("InterfaceByName error: %w", err)
	}

	// Set up ARP client with socket
	c.conn, err = raw.Dial(ifi, syscall.ETH_P_ALL)
	if err != nil {
		return nil, fmt.Errorf("ARP dial error: %w", err)
	}

	return c, nil
}

// NewTestHandler allow you to pass a PacketConn. Useful for testing
// if p is nil, auto create a bufferedPacketConn
func NewTestHandler(config Config, p net.PacketConn) (*Handler, net.PacketConn, error) {
	c := newHandler(config)
	c.table = newARPTable() // we want an empty table for testing
	client, server := raw.NewBufferedConn()
	c.conn = server

	return c, client, nil
}

func newHandler(config Config) (c *Handler) {
	c = &Handler{}
	c.table, _ = loadARPProcTable() // load linux proc table
	c.config.NIC = config.NIC
	c.config.HostMAC = config.HostMAC
	c.config.HostIP = config.HostIP.To4()
	c.config.RouterIP = config.RouterIP.To4()
	c.config.HomeLAN = config.HomeLAN
	c.config.FullNetworkScanInterval = config.FullNetworkScanInterval
	c.config.ProbeInterval = config.ProbeInterval
	c.config.OfflineDeadline = config.OfflineDeadline
	c.config.PurgeDeadline = config.PurgeDeadline

	if c.config.FullNetworkScanInterval <= 0 || c.config.FullNetworkScanInterval > time.Hour*12 {
		c.config.FullNetworkScanInterval = time.Minute * 60
	}
	if c.config.ProbeInterval <= 0 || c.config.ProbeInterval > time.Minute*10 {
		c.config.ProbeInterval = time.Minute * 2
	}
	if c.config.OfflineDeadline <= c.config.ProbeInterval {
		c.config.OfflineDeadline = c.config.ProbeInterval * 2
	}
	if c.config.PurgeDeadline <= c.config.OfflineDeadline {
		c.config.PurgeDeadline = time.Minute * 61
	}

	if Debug {
		log.Printf("ARP Config %s", c.config)
		c.PrintTable()
	}

	return c
}

// AddNotificationChannel set the notification channel for when the MACEntry
// change state between online and offline.
func (c *Handler) AddNotificationChannel(notification chan<- MACEntry) {
	c.notification = notification

	c.Lock()
	table := c.table.getTable()
	c.Unlock()
	go func() {
		for i := range table {
			c.notification <- table[i]
		}
	}()
}

// FindMAC returns a MACEntry or empty if not found
func (c *Handler) FindMAC(mac net.HardwareAddr) (entry MACEntry, found bool) {
	c.RLock()
	defer c.RUnlock()

	e := c.table.findByMAC(mac)
	if e == nil {
		return MACEntry{}, false
	}
	return *e, true
}

// FindIP returns a MACEntry or empty if not found
func (c *Handler) FindIP(ip net.IP) (entry MACEntry, found bool) {
	c.RLock()
	defer c.RUnlock()

	e := c.table.findByIP(ip)
	if e == nil {
		return MACEntry{}, false
	}
	return *e, true
}

// PrintTable print the ARP table to stdout.
func (c *Handler) PrintTable() {
	c.RLock()
	defer c.RUnlock()

	log.Printf("ARP Table: %v entries", len(c.table.macTable))
	c.table.printTable()
}

// GetTable return the mac table as a shallow array of MACEntry
func (c *Handler) GetTable() []MACEntry {
	return c.table.getTable()
}

// Close will terminate the ListenAndServer goroutine as well as all other pending goroutines.
func (c *Handler) Close() {
	// Close the arp socket
	c.conn.Close()
}

// ListenAndServe listen for ARP packets and action each.
//
// When a new MAC is detected, it is automatically added to the ARP table and marked as online.
// Use packet buffer and selectivelly copy mac and ip if we need to keep it
//
// Online and offline notifications
// It will track when a MAC switch between online and offline and will send a message
// in the notification channel set via AddNotificationChannel(). It will poll each known device
// based on the scanInterval parameter using a unicast ARP request.
//
//
// Virtual MACs
// A virtual MAC is a fake mac address used when claiming an existing IP during spoofing.
// ListenAndServe will send ARP reply on behalf of virtual MACs
//
func (c *Handler) Begin(ctx context.Context) error {

	var wg sync.WaitGroup

	// Set ZERO timeout to block forever
	if err := c.conn.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("ARP error in socket: %w", err)
	}

	if c.config.FullNetworkScanInterval != 0 {
		// continuosly scan for network devices
		go func() {
			wg.Add(1)
			if err := c.scanLoop(ctx, c.config.FullNetworkScanInterval); err != nil {
				log.Print("ARP goroutine scanLoop terminated unexpectedly", err)
				c.Close() // force error in main loop
			}
			wg.Done()
			if Debug {
				log.Print("ARP goroutine scanLoop ended")
			}
		}()
	}

	// continously probe for online reply
	go func() {
		wg.Add(1)
		if err := c.probeOnlineLoop(ctx, c.config.ProbeInterval); err != nil {
			log.Print("ARP goroutine probeOnlineLoop terminated unexpectedly", err)
		}
		c.Close() // close conn to force error in main loopi to finish quickly
		wg.Done()
		if Debug {
			log.Print("ARP goroutine probeOnlineLoop ended")
		}
	}()

	// continously check for online-offline transition
	go func() {
		wg.Add(1)
		if err := c.purgeLoop(ctx, c.config.OfflineDeadline, c.config.PurgeDeadline); err != nil {
			log.Print("ARP ListenAndServer purgeLoop terminated unexpectedly", err)
			c.Close() // force error in main loop
		}
		wg.Done()
		if Debug {
			log.Print("ARP goroutine purgeLoop ended")
		}
	}()

	// Do a full scan on start
	if c.config.FullNetworkScanInterval != 0 {
		go func() {
			time.Sleep(time.Millisecond * 100) // Time to start read loop below
			if err := c.ScanNetwork(ctx, c.config.HomeLAN); err != nil {
				log.Print("ARP ListenAndServer scanNetwork terminated unexpectedly", err)
				c.Close() // force error in main loop
			}
			if Debug {
				log.Print("ARP goroutine scanNetwork ended")
			}
		}()
	}

	wg.Wait()
	return nil
}

func (c *Handler) ProcessPacket(host *packet.Host, b []byte) error {
	notify := 0

	frame := ARP(b)
	fmt.Printf("arp  : %s\n", frame)
	if !frame.IsValid() {
		return packet.ErrParseMessage
	}
	if Debug {
		fmt.Printf("arp  : %s\n", frame)
	}

	// skip link local packets
	if frame.SrcIP().IsLinkLocalUnicast() || frame.DstIP().IsLinkLocalUnicast() {
		if Debug {
			log.Printf("ARP skipping link local packet smac=%v sip=%v tmac=%v tip=%v", frame.SrcMAC(), frame.SrcIP(), frame.DstMAC(), frame.DstIP())
		}
		return nil
	}

	if Debug {
		switch {
		case frame.Operation() == OperationReply:
			log.Printf("ARP reply recvd: %s", frame)
		case frame.Operation() == OperationRequest:
			switch {
			case frame.SrcIP().Equal(frame.DstIP()):
				log.Printf("ARP announcement recvd: %s", frame)
			case frame.SrcIP().Equal(net.IPv4zero):
				log.Printf("ARP probe recvd: %s", frame)
			default:
				log.Printf("ARP who is %s: %s ", frame.DstIP(), frame)
			}
		default:
			log.Printf("ARP invalid operation: %s", frame)
			return nil
		}
	}

	// Ignore router packets
	if bytes.Equal(frame.SrcIP(), c.config.RouterIP) {
		if c.routerEntry.MAC == nil { // store router MAC
			c.routerEntry.MAC = dupMAC(frame.SrcMAC())
			c.routerEntry.IPArray[0] = IPEntry{IP: c.config.RouterIP}
		}
		return nil
	}

	// Ignore host packets
	if bytes.Equal(frame.SrcMAC(), c.config.HostMAC) {
		return nil
	}

	// if targetIP is a virtual host, we are claiming the ip; reply and return
	c.RLock()
	if target := c.table.findVirtualIP(frame.DstIP()); target != nil {
		mac := target.MAC
		c.RUnlock()
		if Debug {
			log.Printf("ARP ip=%s is virtual - send reply smac=%v", frame.DstIP(), mac)
		}
		c.reply(frame.SrcMAC(), mac, frame.DstIP(), EthernetBroadcast, frame.DstIP())
		return nil
	}
	c.RUnlock()

	// We are not interested in probe ACD (Address Conflict Detection) packets
	// if this is a probe, the sender IP will be zeros; do nothing as the sender IP is not valid yet.
	//
	// +============+===+===========+===========+============+============+===================+===========+
	// | Type       | op| dstMAC    | srcMAC    | SenderMAC  | SenderIP   | TargetMAC         |  TargetIP |
	// +============+===+===========+===========+============+============+===================+===========+
	// | ACD probe  | 1 | broadcast | clientMAC | clientMAC  | 0x00       | 0x00              |  targetIP |
	// | ACD announ | 1 | broadcast | clientMAC | clientMAC  | clientIP   | ff:ff:ff:ff:ff:ff |  clientIP |
	// +============+===+===========+===========+============+============+===================+===========+
	if frame.SrcIP().Equal(net.IPv4zero) {
		return nil
	}

	c.Lock()
	defer c.Unlock()

	sender := c.table.findByMAC(frame.SrcMAC())
	if sender == nil {
		// If new client, then create a MACEntry in table
		sender, _ = c.table.upsert(StateNormal, dupMAC(frame.SrcMAC()), dupIP(frame.SrcIP()))
		notify++
	} else {
		// notify online transition
		if sender.Online == false {
			notify++
		}
	}

	// Skip packets that we sent as virtual host (i.e. we sent these)
	if sender.State == StateVirtualHost {
		return nil
	}

	sender.LastUpdated = time.Now()

	switch frame.Operation() {

	case OperationRequest:

		switch sender.State {
		case StateHunt:
			// If this is a new IP, stop hunting it.
			// The spoof goroutine will detect the mac changed to normal and terminate.
			if !c.table.updateIP(sender, dupIP(frame.SrcIP())) {
				sender.State = StateNormal
				notify++
			}

		case StateNormal:
			if !c.table.updateIP(sender, dupIP(frame.SrcIP())) {
				notify++
			}

		default:
			log.Print("ARP unexpected client state in request =", sender.State)
		}

	case OperationReply:
		// Android does not send collision detection request,
		// we will see a reply instead. Check if the address has changed.
		if !c.table.updateIP(sender, dupIP(frame.SrcIP())) {
			sender.State = StateNormal // will end hunt goroutine
			notify++
		}
	}

	if notify > 0 {
		if sender.Online == false {
			sender.Online = true
			log.Printf("ARP ip=%s is online mac=%s state=%s ips=%s", frame.SrcIP(), sender.MAC, sender.State, sender.IPs())
		} else {
			log.Printf("ARP ip=%s is online - updated ip for mac=%s state=%s ips=%s", frame.SrcIP(), sender.MAC, sender.State, sender.IPs())
		}

		if c.notification != nil {
			c.notification <- *sender
		}
	}

	return nil
}
