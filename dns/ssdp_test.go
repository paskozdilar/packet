package dns

import (
	"fmt"
	"testing"

	"github.com/irai/packet"
)

// b8:e9:37:51:89:8c > ff:ff:ff:ff:ff:ff, ethertype IPv4 (0x0800), length 530: (tos 0x0, ttl 64, id 0, offset 0, flags [DF], proto UDP (17), length 516)
// 192.168.0.103.54582 > 255.255.255.255.1900: [udp sum ok] UDP, length 488
var ssdpFrame = []byte{
	0x45, 0x00, 0x02, 0x04, 0x00, 0x00, 0x40, 0x00, 0x40, 0x11, 0x77, 0xda, 0xc0, 0xa8, 0x00, 0x67, //  E.....@.@.w....g
	0xff, 0xff, 0xff, 0xff, 0xd5, 0x36, 0x07, 0x6c, 0x01, 0xf0, 0xe9, 0xe3, 0x4e, 0x4f, 0x54, 0x49, //  .....6.l....NOTI
	0x46, 0x59, 0x20, 0x2a, 0x20, 0x48, 0x54, 0x54, 0x50, 0x2f, 0x31, 0x2e, 0x31, 0x0d, 0x0a, 0x48, //  FY.*.HTTP/1.1..H
	0x4f, 0x53, 0x54, 0x3a, 0x20, 0x32, 0x33, 0x39, 0x2e, 0x32, 0x35, 0x35, 0x2e, 0x32, 0x35, 0x35, //  OST:.239.255.255
	0x2e, 0x32, 0x35, 0x30, 0x3a, 0x31, 0x39, 0x30, 0x30, 0x0d, 0x0a, 0x43, 0x41, 0x43, 0x48, 0x45, //  .250:1900..CACHE
	0x2d, 0x43, 0x4f, 0x4e, 0x54, 0x52, 0x4f, 0x4c, 0x3a, 0x20, 0x6d, 0x61, 0x78, 0x2d, 0x61, 0x67, //  -CONTROL:.max-ag
	0x65, 0x20, 0x3d, 0x20, 0x31, 0x38, 0x30, 0x30, 0x0d, 0x0a, 0x4c, 0x4f, 0x43, 0x41, 0x54, 0x49, //  e.=.1800..LOCATI
	0x4f, 0x4e, 0x3a, 0x20, 0x68, 0x74, 0x74, 0x70, 0x3a, 0x2f, 0x2f, 0x31, 0x39, 0x32, 0x2e, 0x31, //  ON:.http://192.1
	0x36, 0x38, 0x2e, 0x30, 0x2e, 0x31, 0x30, 0x33, 0x3a, 0x31, 0x34, 0x30, 0x30, 0x2f, 0x78, 0x6d, //  68.0.103:1400/xm
	0x6c, 0x2f, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x5f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, //  l/device_descrip
	0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x78, 0x6d, 0x6c, 0x0d, 0x0a, 0x4e, 0x54, 0x3a, 0x20, 0x75, 0x70, //  tion.xml..NT:.up
	0x6e, 0x70, 0x3a, 0x72, 0x6f, 0x6f, 0x74, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x0d, 0x0a, 0x4e, //  np:rootdevice..N
	0x54, 0x53, 0x3a, 0x20, 0x73, 0x73, 0x64, 0x70, 0x3a, 0x61, 0x6c, 0x69, 0x76, 0x65, 0x0d, 0x0a, //  TS:.ssdp:alive..
	0x53, 0x45, 0x52, 0x56, 0x45, 0x52, 0x3a, 0x20, 0x4c, 0x69, 0x6e, 0x75, 0x78, 0x20, 0x55, 0x50, //  SERVER:.Linux.UP
	0x6e, 0x50, 0x2f, 0x31, 0x2e, 0x30, 0x20, 0x53, 0x6f, 0x6e, 0x6f, 0x73, 0x2f, 0x35, 0x37, 0x2e, //  nP/1.0.Sonos/57.
	0x33, 0x2d, 0x37, 0x39, 0x32, 0x30, 0x30, 0x20, 0x28, 0x5a, 0x50, 0x53, 0x31, 0x29, 0x0d, 0x0a, //  3-79200.(ZPS1)..
	0x55, 0x53, 0x4e, 0x3a, 0x20, 0x75, 0x75, 0x69, 0x64, 0x3a, 0x52, 0x49, 0x4e, 0x43, 0x4f, 0x4e, //  USN:.uuid:RINCON
	0x5f, 0x42, 0x38, 0x45, 0x39, 0x33, 0x37, 0x35, 0x31, 0x38, 0x39, 0x38, 0x43, 0x30, 0x31, 0x34, //  _B8E93751898C014
	0x30, 0x30, 0x3a, 0x3a, 0x75, 0x70, 0x6e, 0x70, 0x3a, 0x72, 0x6f, 0x6f, 0x74, 0x64, 0x65, 0x76, //  00::upnp:rootdev
	0x69, 0x63, 0x65, 0x0d, 0x0a, 0x58, 0x2d, 0x52, 0x49, 0x4e, 0x43, 0x4f, 0x4e, 0x2d, 0x48, 0x4f, //  ice..X-RINCON-HO
	0x55, 0x53, 0x45, 0x48, 0x4f, 0x4c, 0x44, 0x3a, 0x20, 0x53, 0x6f, 0x6e, 0x6f, 0x73, 0x5f, 0x30, //  USEHOLD:.Sonos_0
	0x46, 0x46, 0x6c, 0x35, 0x44, 0x74, 0x61, 0x6e, 0x59, 0x50, 0x52, 0x67, 0x65, 0x32, 0x50, 0x46, //  FFl5DtanYPRge2PF
	0x7a, 0x35, 0x77, 0x55, 0x6c, 0x45, 0x48, 0x6f, 0x72, 0x0d, 0x0a, 0x58, 0x2d, 0x52, 0x49, 0x4e, //  z5wUlEHor..X-RIN
	0x43, 0x4f, 0x4e, 0x2d, 0x42, 0x4f, 0x4f, 0x54, 0x53, 0x45, 0x51, 0x3a, 0x20, 0x33, 0x30, 0x30, //  CON-BOOTSEQ:.300
	0x0d, 0x0a, 0x58, 0x2d, 0x52, 0x49, 0x4e, 0x43, 0x4f, 0x4e, 0x2d, 0x57, 0x49, 0x46, 0x49, 0x4d, //  ..X-RINCON-WIFIM
	0x4f, 0x44, 0x45, 0x3a, 0x20, 0x30, 0x0d, 0x0a, 0x58, 0x2d, 0x52, 0x49, 0x4e, 0x43, 0x4f, 0x4e, //  ODE:.0..X-RINCON
	0x2d, 0x56, 0x41, 0x52, 0x49, 0x41, 0x4e, 0x54, 0x3a, 0x20, 0x30, 0x0d, 0x0a, 0x48, 0x4f, 0x55, //  -VARIANT:.0..HOU
	0x53, 0x45, 0x48, 0x4f, 0x4c, 0x44, 0x2e, 0x53, 0x4d, 0x41, 0x52, 0x54, 0x53, 0x50, 0x45, 0x41, //  SEHOLD.SMARTSPEA
	0x4b, 0x45, 0x52, 0x2e, 0x41, 0x55, 0x44, 0x49, 0x4f, 0x3a, 0x20, 0x53, 0x6f, 0x6e, 0x6f, 0x73, //  KER.AUDIO:.Sonos
	0x5f, 0x30, 0x46, 0x46, 0x6c, 0x35, 0x44, 0x74, 0x61, 0x6e, 0x59, 0x50, 0x52, 0x67, 0x65, 0x32, //  _0FFl5DtanYPRge2
	0x50, 0x46, 0x7a, 0x35, 0x77, 0x55, 0x6c, 0x45, 0x48, 0x6f, 0x72, 0x2e, 0x64, 0x46, 0x2d, 0x44, //  PFz5wUlEHor.dF-D
	0x70, 0x34, 0x6b, 0x54, 0x53, 0x30, 0x46, 0x41, 0x35, 0x35, 0x70, 0x78, 0x4a, 0x42, 0x65, 0x38, //  p4kTS0FA55pxJBe8
	0x0d, 0x0a, 0x0d, 0x0a, //  ....
}

func TestDNSHandler_ProcessSSDP(t *testing.T) {
	session := packet.NewEmptySession()
	dnsHandler, _ := New(session)

	Debug = true
	tests := []struct {
		name         string
		frame        []byte
		wantLocation string
		wantErr      bool
	}{
		{name: "ssdp1", frame: ssdpFrame, wantErr: false, wantLocation: "http://192.168.0.103:1400/xml/device_description.xml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := packet.IP4(tt.frame)
			fmt.Println("ip", ip)
			udp := packet.UDP(ip.Payload())
			fmt.Println("udp", udp)
			location, err := dnsHandler.ProcessSSDP(nil, nil, udp.Payload())
			if (err != nil) != tt.wantErr {
				t.Errorf("DNSHandler.ProcessSSDP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if location != tt.wantLocation {
				t.Errorf("DNSHandler.ProcessSSDP() invalid name = %+v, want %v", location, tt.wantLocation)
			}
		})
	}
}

var serviceDefinitionSonos = []byte(`
<root xmlns="urn:schemas-upnp-org:device-1-0">
<specVersion>
<major>1</major>
<minor>0</minor>
</specVersion>
<device>
<deviceType>urn:schemas-upnp-org:device:ZonePlayer:1</deviceType>
<friendlyName>192.168.0.103 - Sonos Play:1</friendlyName>
<manufacturer>Sonos, Inc.</manufacturer>
<manufacturerURL>http://www.sonos.com</manufacturerURL>
<modelNumber>S1</modelNumber>
<modelDescription>Sonos Play:1</modelDescription>
<modelName>Sonos Play:1</modelName>
<modelURL>http://www.sonos.com/products/zoneplayers/S1</modelURL>
<softwareVersion>57.3-79200</softwareVersion>
<swGen>1</swGen>
<hardwareVersion>1.8.3.7-2.0</hardwareVersion>
<serialNum>B8-E9-37-51-89-8C:5</serialNum>
<MACAddress>B8:E9:37:51:89:8C</MACAddress>
<UDN>uuid:RINCON_B8E93751898C01400</UDN>
<iconList>
<icon>
<id>0</id>
<mimetype>image/png</mimetype>
<width>48</width>
<height>48</height>
<depth>24</depth>
<url>/img/icon-S1.png</url>
</icon>
</iconList>
<minCompatibleVersion>56.0-00000</minCompatibleVersion>
<legacyCompatibleVersion>36.0-00000</legacyCompatibleVersion>
<apiVersion>1.18.9</apiVersion>
<minApiVersion>1.1.0</minApiVersion>
<displayVersion>11.2.2</displayVersion>
<extraVersion>OTP: </extraVersion>
<roomName>Kitchen</roomName>
<displayName>Play:1</displayName>
<zoneType>9</zoneType>
<feature1>0x00000000</feature1>
<feature2>0x00403332</feature2>
<feature3>0x0001100e</feature3>
<seriesid>A101</seriesid>
<variant>0</variant>
<internalSpeakerSize>5</internalSpeakerSize>
<bassExtension>75.000</bassExtension>
<satGainOffset>6.000</satGainOffset>
<memory>128</memory>
<flash>64</flash>
#DEACTIVATION_STATE_TAG_AND_VALUE# #DEACTIVATION_TTL_TAG_AND_VALUE# #DEACTIVATION_DATE_TIME_TAG_AND_VALUE#
<ampOnTime>10</ampOnTime>
<retailMode>0</retailMode>
<serviceList>
<service>
<serviceType>urn:schemas-upnp-org:service:AlarmClock:1</serviceType>
<serviceId>urn:upnp-org:serviceId:AlarmClock</serviceId>
<controlURL>/AlarmClock/Control</controlURL>
<eventSubURL>/AlarmClock/Event</eventSubURL>
<SCPDURL>/xml/AlarmClock1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-upnp-org:service:MusicServices:1</serviceType>
<serviceId>urn:upnp-org:serviceId:MusicServices</serviceId>
<controlURL>/MusicServices/Control</controlURL>
<eventSubURL>/MusicServices/Event</eventSubURL>
<SCPDURL>/xml/MusicServices1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-upnp-org:service:DeviceProperties:1</serviceType>
<serviceId>urn:upnp-org:serviceId:DeviceProperties</serviceId>
<controlURL>/DeviceProperties/Control</controlURL>
<eventSubURL>/DeviceProperties/Event</eventSubURL>
<SCPDURL>/xml/DeviceProperties1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-upnp-org:service:SystemProperties:1</serviceType>
<serviceId>urn:upnp-org:serviceId:SystemProperties</serviceId>
<controlURL>/SystemProperties/Control</controlURL>
<eventSubURL>/SystemProperties/Event</eventSubURL>
<SCPDURL>/xml/SystemProperties1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-upnp-org:service:ZoneGroupTopology:1</serviceType>
<serviceId>urn:upnp-org:serviceId:ZoneGroupTopology</serviceId>
<controlURL>/ZoneGroupTopology/Control</controlURL>
<eventSubURL>/ZoneGroupTopology/Event</eventSubURL>
<SCPDURL>/xml/ZoneGroupTopology1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-upnp-org:service:GroupManagement:1</serviceType>
<serviceId>urn:upnp-org:serviceId:GroupManagement</serviceId>
<controlURL>/GroupManagement/Control</controlURL>
<eventSubURL>/GroupManagement/Event</eventSubURL>
<SCPDURL>/xml/GroupManagement1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-tencent-com:service:QPlay:1</serviceType>
<serviceId>urn:tencent-com:serviceId:QPlay</serviceId>
<controlURL>/QPlay/Control</controlURL>
<eventSubURL>/QPlay/Event</eventSubURL>
<SCPDURL>/xml/QPlay1.xml</SCPDURL>
</service>
</serviceList>
<deviceList>
<device>
<deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType>
<friendlyName>192.168.0.103 - Sonos Play:1 Media Server</friendlyName>
<manufacturer>Sonos, Inc.</manufacturer>
<manufacturerURL>http://www.sonos.com</manufacturerURL>
<modelNumber>S1</modelNumber>
<modelDescription>Sonos Play:1 Media Server</modelDescription>
<modelName>Sonos Play:1</modelName>
<modelURL>http://www.sonos.com/products/zoneplayers/S1</modelURL>
<UDN>uuid:RINCON_B8E93751898C01400_MS</UDN>
<serviceList>
<service>
<serviceType>urn:schemas-upnp-org:service:ContentDirectory:1</serviceType>
<serviceId>urn:upnp-org:serviceId:ContentDirectory</serviceId>
<controlURL>/MediaServer/ContentDirectory/Control</controlURL>
<eventSubURL>/MediaServer/ContentDirectory/Event</eventSubURL>
<SCPDURL>/xml/ContentDirectory1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType>
<serviceId>urn:upnp-org:serviceId:ConnectionManager</serviceId>
<controlURL>/MediaServer/ConnectionManager/Control</controlURL>
<eventSubURL>/MediaServer/ConnectionManager/Event</eventSubURL>
<SCPDURL>/xml/ConnectionManager1.xml</SCPDURL>
</service>
</serviceList>
</device>
<device>
<deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
<friendlyName>Kitchen - Sonos Play:1 Media Renderer</friendlyName>
<manufacturer>Sonos, Inc.</manufacturer>
<manufacturerURL>http://www.sonos.com</manufacturerURL>
<modelNumber>S1</modelNumber>
<modelDescription>Sonos Play:1 Media Renderer</modelDescription>
<modelName>Sonos Play:1</modelName>
<modelURL>http://www.sonos.com/products/zoneplayers/S1</modelURL>
<UDN>uuid:RINCON_B8E93751898C01400_MR</UDN>
<serviceList>
<service>
<serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType>
<serviceId>urn:upnp-org:serviceId:RenderingControl</serviceId>
<controlURL>/MediaRenderer/RenderingControl/Control</controlURL>
<eventSubURL>/MediaRenderer/RenderingControl/Event</eventSubURL>
<SCPDURL>/xml/RenderingControl1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType>
<serviceId>urn:upnp-org:serviceId:ConnectionManager</serviceId>
<controlURL>/MediaRenderer/ConnectionManager/Control</controlURL>
<eventSubURL>/MediaRenderer/ConnectionManager/Event</eventSubURL>
<SCPDURL>/xml/ConnectionManager1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
<serviceId>urn:upnp-org:serviceId:AVTransport</serviceId>
<controlURL>/MediaRenderer/AVTransport/Control</controlURL>
<eventSubURL>/MediaRenderer/AVTransport/Event</eventSubURL>
<SCPDURL>/xml/AVTransport1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-sonos-com:service:Queue:1</serviceType>
<serviceId>urn:sonos-com:serviceId:Queue</serviceId>
<controlURL>/MediaRenderer/Queue/Control</controlURL>
<eventSubURL>/MediaRenderer/Queue/Event</eventSubURL>
<SCPDURL>/xml/Queue1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-upnp-org:service:GroupRenderingControl:1</serviceType>
<serviceId>urn:upnp-org:serviceId:GroupRenderingControl</serviceId>
<controlURL>/MediaRenderer/GroupRenderingControl/Control</controlURL>
<eventSubURL>/MediaRenderer/GroupRenderingControl/Event</eventSubURL>
<SCPDURL>/xml/GroupRenderingControl1.xml</SCPDURL>
</service>
<service>
<serviceType>urn:schemas-upnp-org:service:VirtualLineIn:1</serviceType>
<serviceId>urn:upnp-org:serviceId:VirtualLineIn</serviceId>
<controlURL>/MediaRenderer/VirtualLineIn/Control</controlURL>
<eventSubURL>/MediaRenderer/VirtualLineIn/Event</eventSubURL>
<SCPDURL>/xml/VirtualLineIn1.xml</SCPDURL>
</service>
</serviceList>
<X_Rhapsody-Extension xmlns="http://www.real.com/rhapsody/xmlns/upnp-1-0">
<deviceID>urn:rhapsody-real-com:device-id-1-0:sonos_1:RINCON_B8E93751898C01400</deviceID>
<deviceCapabilities>
<interactionPattern type="real-rhapsody-upnp-1-0"/>
</deviceCapabilities>
</X_Rhapsody-Extension>
<qq:X_QPlay_SoftwareCapability xmlns:qq="http://www.tencent.com">QPlay:2</qq:X_QPlay_SoftwareCapability>
<iconList>
<icon>
<mimetype>image/png</mimetype>
<width>48</width>
<height>48</height>
<depth>24</depth>
<url>/img/icon-S1.png</url>
</icon>
</iconList>
</device>
</deviceList>
</device>
</root>
`)

var serviceDefinitionTPLink = []byte(`
<root xmlns="urn:schemas-upnp-org:device-1-0">
<specVersion>
<major>1</major>
<minor>0</minor>
</specVersion>
<device>
<deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
<presentationURL>http://192.168.0.1:80/</presentationURL>
<friendlyName>Archer_VR1600v</friendlyName>
<manufacturer>TP-Link</manufacturer>
<manufacturerURL>http://www.tp-link.com</manufacturerURL>
<modelDescription>AC1600 Wireless Dual Band Gigabit VoIP VDSL/ADSL Modem Router</modelDescription>
<modelName>Archer_VR1600v</modelName>
<modelNumber>1.0</modelNumber>
<modelURL>http://192.168.0.1:80/</modelURL>
<serialNumber>1.0</serialNumber>
<UDN>uuid:9f0865b3-f5da-4ad5-85b7-7404637fdf37</UDN>
<serviceList>
<service>
<serviceType>urn:schemas-upnp-org:service:Layer3Forwarding:1</serviceType>
<serviceId>urn:upnp-org:serviceId:L3Forwarding1</serviceId>
<controlURL>/upnp/control/dummy</controlURL>
<eventSubURL>/upnp/control/dummy</eventSubURL>
<SCPDURL>/dummy.xml</SCPDURL>
</service>
</serviceList>
<deviceList>
<device>
<deviceType>urn:schemas-upnp-org:device:WANDevice:1</deviceType>
<friendlyName>Archer_VR1600v</friendlyName>
<manufacturer>TP-Link</manufacturer>
<manufacturerURL>http://www.tp-link.com</manufacturerURL>
<modelDescription>AC1600 Wireless Dual Band Gigabit VoIP VDSL/ADSL Modem Router</modelDescription>
<modelName>Archer_VR1600v</modelName>
<modelNumber>1.0</modelNumber>
<modelURL>http://192.168.0.1:80/</modelURL>
<serialNumber>1.0</serialNumber>
<UDN>uuid:9f0865b3-f5da-4ad5-85b7-7404637fdf38</UDN>
<serviceList>
<service>
<serviceType>urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1</serviceType>
<serviceId>urn:upnp-org:serviceId:WANCommonIFC1</serviceId>
<controlURL>/upnp/control/WANCommonIFC1</controlURL>
<eventSubURL>/upnp/control/WANCommonIFC1</eventSubURL>
<SCPDURL>/gateicfgSCPD.xml</SCPDURL>
</service>
</serviceList>
<deviceList>
<device>
<deviceType>urn:schemas-upnp-org:device:WANConnectionDevice:1</deviceType>
<friendlyName>Archer_VR1600v</friendlyName>
<manufacturer>TP-Link</manufacturer>
<manufacturerURL>http://www.tp-link.com</manufacturerURL>
<modelDescription>AC1600 Wireless Dual Band Gigabit VoIP VDSL/ADSL Modem Router</modelDescription>
<modelName>Archer_VR1600v</modelName>
<modelNumber>1.0</modelNumber>
<modelURL>http://192.168.0.1:80/</modelURL>
<serialNumber>1.0</serialNumber>
<UDN>uuid:9f0865b3-f5da-4ad5-85b7-7404637fdf39</UDN>
<serviceList>
<service>
<serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>
<serviceId>urn:upnp-org:serviceId:WANIPConn1</serviceId>
<controlURL>/upnp/control/WANIPConn1</controlURL>
<eventSubURL>/upnp/control/WANIPConn1</eventSubURL>
<SCPDURL>/gateconnSCPD.xml</SCPDURL>
</service>
</serviceList>
</device>
</deviceList>
</device>
</deviceList>
</device>
</root>
`)

func TestDNSHandler_ProcessSSDPDescription(t *testing.T) {
	Debug = true
	tests := []struct {
		name      string
		service   []byte
		wantErr   bool
		wantName  string
		wantModel string
	}{
		{name: "ssdp-sonos", service: serviceDefinitionSonos, wantErr: false, wantName: "192.168.0.103 - Sonos Play:1", wantModel: "Sonos Play:1"},
		{name: "ssdp-tplink", service: serviceDefinitionTPLink, wantErr: false, wantName: "Archer_VR1600v", wantModel: "Archer_VR1600v"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := UnmarshalSSDPService(tt.service)
			if (err != nil) != tt.wantErr {
				t.Errorf("DNSHandler.ProcessSSDPDescription() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if v.Device.Name != tt.wantName {
				t.Errorf("DNSHandler.ProcessSSDPDescription() name=%v, want=%v", v.Device.Name, tt.wantName)
				return
			}
			if v.Device.Model != tt.wantModel {
				t.Errorf("DNSHandler.ProcessSSDPDescription() model=%v, want=%v", v.Device.Model, tt.wantModel)
				return
			}
		})
	}
}
