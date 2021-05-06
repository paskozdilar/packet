package packet

import (
	"fmt"
	"net"
	"time"

	"github.com/irai/packet/model"
)

type Notification struct {
	Addr     model.Addr
	Online   bool
	DHCPName string
	MDNSName string
}

func (n Notification) String() string {
	return fmt.Sprintf("%s online=%t dhcp4name=%s mdnsname=%s", n.Addr, n.Online, n.DHCPName, n.MDNSName)
}

// purge is called each minute by the minute goroutine
func (h *Handler) purge(now time.Time, probeDur time.Duration, offlineDur time.Duration, purgeDur time.Duration) error {

	probeCutoff := now.Add(probeDur * -1)     // Mark offline entries last updated before this time
	offlineCutoff := now.Add(offlineDur * -1) // Mark offline entries last updated before this time
	deleteCutoff := now.Add(purgeDur * -1)    // Delete entries that have not responded in last hour

	purge := make([]net.IP, 0, 16)
	offline := make([]*model.Host, 0, 16)

	h.mutex.RLock()
	for _, e := range h.session.HostTable.Table {
		e.Row.RLock()

		// Delete from table if the device is offline and was not seen for the last hour
		if !e.Online && e.LastSeen.Before(deleteCutoff) {
			purge = append(purge, e.IP)
			e.Row.RUnlock()
			continue
		}

		// Probe if device not seen recently
		if e.Online && e.LastSeen.Before(probeCutoff) {
			if ip := e.IP.To4(); ip != nil {
				h.HandlerARP.CheckAddr(model.Addr{MAC: e.MACEntry.MAC, IP: ip})
			} else {
				h.HandlerICMP6.CheckAddr(model.Addr{MAC: e.MACEntry.MAC, IP: e.IP})
			}
		}

		// Set offline if no updates since the offline deadline
		if e.Online && e.LastSeen.Before(offlineCutoff) {
			offline = append(offline, e)
		}
		e.Row.RUnlock()
	}
	h.mutex.RUnlock()

	for _, host := range offline {
		h.lockAndSetOffline(host) // will lock/unlock row
	}

	// delete after loop because this will change the table
	if len(purge) > 0 {
		for _, v := range purge {
			h.session.DeleteHost(v)
		}
	}

	return nil
}

func (h *Handler) GetNotificationChannel() <-chan Notification {
	if h.nameChannel != nil {
		return h.nameChannel
	}

	// Notify of all existing hosts
	list := []Notification{}
	h.mutex.RLock()
	for _, host := range h.session.HostTable.Table {
		host.Row.RLock()
		list = append(list, Notification{Addr: model.Addr{IP: host.IP, MAC: host.MACEntry.MAC}, Online: host.Online, DHCPName: host.DHCP4Name})
		host.Row.RUnlock()
	}
	h.mutex.RUnlock()

	h.nameChannel = make(chan Notification, notificationChannelCap)

	go func() {
		for _, n := range list {
			h.nameChannel <- n
			time.Sleep(time.Millisecond * 5) // time for reader to process
		}
	}()

	return h.nameChannel
}
