package icmp4

import (
	"fmt"
	"syscall"

	"github.com/irai/packet"
)

func (h *Handler) sendPacket(srcAddr packet.Addr, dstAddr packet.Addr, p packet.ICMP4) (err error) {
	ether := packet.Ether(make([]byte, packet.EthMaxSize))
	ether = packet.EtherMarshalBinary(ether, syscall.ETH_P_IP, h.session.NICInfo.HostMAC, dstAddr.MAC)
	ip4 := packet.IP4MarshalBinary(ether.Payload(), 50, srcAddr.IP, dstAddr.IP)
	if ip4, err = ip4.AppendPayload(p, syscall.IPPROTO_ICMP); err != nil {
		return err
	}
	if ether, err = ether.SetPayload(ip4); err != nil {
		return err
	}
	if Debug {
		fmt.Printf("icmp4 : send %s\n", p)
	}
	if _, err := h.session.Conn.WriteTo(ether, &dstAddr); err != nil {
		fmt.Println("icmp4 : error sending packet ", dstAddr, err)
		return err
	}
	return nil
}
