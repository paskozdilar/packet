package icmp

import (
	"fmt"
	"testing"

	"github.com/irai/packet"
	"inet.af/netaddr"
)

func Test_IP6Lib(t *testing.T) {

	ip, err := netaddr.ParseIP("2001:4479:1d01:2401::")

	if err != nil {
		t.Error("invalid IP ", err)
	}
	fmt.Println(ip)
}

func TestICMP4Redirect_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		p       []byte
		wantErr bool
	}{
		/**
		TODO: capture ICMP4 redirect test frames. these are wrong
		{name: "redirect", wantErr: false, p: []byte{0x74, 0x79, 0x70, 0x65, 0x3d, 0x39, 0x20, 0x63, 0x6f, 0x64, 0x65, 0x3d, 0x30, 0x20, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64, 0x4c, 0x65, 0x6e, 0x3d, 0x31, 0x38, 0x2c, 0x20, 0x64, 0x61, 0x74, 0x61, 0x3d, 0x30, 0x78, 0x63, 0x30, 0x20, 0x61, 0x38, 0x20, 0x30, 0x30, 0x20, 0x30, 0x31, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30}},
		{name: "redirect", wantErr: false, p: []byte{0x74, 0x79, 0x70, 0x65, 0x3d, 0x39, 0x20, 0x63, 0x6f, 0x64, 0x65, 0x3d, 0x30, 0x20, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64, 0x4c, 0x65, 0x6e, 0x3d, 0x31, 0x38, 0x2c, 0x20, 0x64, 0x61, 0x74, 0x61, 0x3d, 0x30, 0x78, 0x63, 0x30, 0x20, 0x61, 0x38, 0x20, 0x30, 0x30, 0x20, 0x30, 0x31, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30}},
		{name: "redirect", wantErr: false, p: []byte{0x74, 0x79, 0x70, 0x65, 0x3d, 0x39, 0x20, 0x63, 0x6f, 0x64, 0x65, 0x3d, 0x30, 0x20, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64, 0x4c, 0x65, 0x6e, 0x3d, 0x31, 0x38, 0x2c, 0x20, 0x64, 0x61, 0x74, 0x61, 0x3d, 0x30, 0x78, 0x32, 0x38, 0x20, 0x30, 0x34, 0x20, 0x30, 0x31, 0x20, 0x34, 0x64, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30}},
		{name: "redirect", wantErr: false, p: []byte{0x74, 0x79, 0x70, 0x65, 0x3d, 0x39, 0x20, 0x63, 0x6f, 0x64, 0x65, 0x3d, 0x30, 0x20, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64, 0x4c, 0x65, 0x6e, 0x3d, 0x31, 0x38, 0x2c, 0x20, 0x64, 0x61, 0x74, 0x61, 0x3d, 0x30, 0x78, 0x32, 0x38, 0x20, 0x30, 0x34, 0x20, 0x30, 0x31, 0x20, 0x34, 0x64, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30, 0x20, 0x30, 0x30}},
		**/
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ether := packet.Ether(tt.p)
			fmt.Println("test icmp redirect", ether)
			ip := packet.IP4(ether.Payload())
			fmt.Println("test icmp redirect", ip)
			p := ICMP4Redirect(ip.Payload())
			if err := p.IsValid(); (err != nil) != tt.wantErr {
				t.Errorf("ICMP4Redirect.IsValid() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			fmt.Println("test icmp redirect", p)
		})
	}
}
