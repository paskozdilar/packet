package arp

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"

	"github.com/irai/packet"
)

// ARP Operation types
const (
	OperationRequest = 1
	OperationReply   = 2
)

// ARP memory mapped arp packet
type ARP []byte

// arpLen length is header + 2 * MACs + 2 IPs
// in theory MAC and IP len can vary
const arpLen = 8 + 2*6 + 2*4

func (b ARP) IsValid() bool {
	if len(b) < arpLen {
		return false
	}
	if b.HType() != 1 {
		return false
	}
	if b.Proto() != syscall.ETH_P_IP {
		return false
	}
	if b.HLen() != 6 {
		return false
	}
	if b.PLen() != 4 {
		return false
	}

	return true
}

func (b ARP) HType() uint16 {
	return binary.BigEndian.Uint16(b[0:2])
}

func (b ARP) Proto() uint16 {
	return binary.BigEndian.Uint16(b[2:4])
}

func (b ARP) HLen() uint8 {
	return b[4]
}

func (b ARP) PLen() uint8 {
	return b[5]
}

func (b ARP) Operation() uint16 {
	return binary.BigEndian.Uint16(b[6:8])
}

func (b ARP) SrcMAC() net.HardwareAddr {
	return net.HardwareAddr(b[8:14])
}

func (b ARP) SrcIP() net.IP {
	return net.IP(b[14:18])
}

func (b ARP) DstMAC() net.HardwareAddr {
	return net.HardwareAddr(b[18:24])
}

func (b ARP) DstIP() net.IP {
	return net.IP(b[24:28])
}

func (b ARP) String() string {
	return fmt.Sprintf("operation=%d proto=%d srcMAC=%s srcIP=%s dstMAC=%s dstIP=%s", b.Operation(), b.Proto(), b.SrcMAC(), b.SrcIP(), b.DstMAC(), b.DstIP())
}

// MarshalBinary creates a wire ARP frame ready for transmission
// see format: https://en.wikipedia.org/wiki/Address_Resolution_Protocol
//
// operation - 1 request, 2 reply
func MarshalBinary(b []byte, operation uint16, srcAddr packet.Addr, dstAddr packet.Addr) (ARP, error) {
	if b == nil {
		b = make([]byte, arpLen)
	}
	if cap(b) < arpLen {
		return nil, fmt.Errorf("invalid len")
	}
	b = b[:arpLen] // change the slice to accomodate the index below in case slice is less than arpLen

	binary.BigEndian.PutUint16(b[0:2], 1)                // Hardware Type - Ethernet is 1
	binary.BigEndian.PutUint16(b[2:4], syscall.ETH_P_IP) // Protocol type - IPv4 0x0800
	b[4] = 6                                             // mac len - fixed
	b[5] = 4                                             // ipv4 len - fixed
	binary.BigEndian.PutUint16(b[6:8], operation)        // operation
	copy(b[8:8+6], srcAddr.MAC[:6])
	copy(b[14:14+4], srcAddr.IP[:4])
	copy(b[18:18+6], dstAddr.MAC[:6])
	copy(b[24:24+4], dstAddr.IP[:4])
	return b, nil
}
