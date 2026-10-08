package app

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"

	"github.com/hashcott/ghostline/internal/store"
)

// DeviceInfo is what the "Use on other devices" card shows.
type DeviceInfo struct {
	DNSAddrs    []string `json:"dnsAddrs"`
	DoHURLs     []string `json:"dohUrls"`
	Fingerprint string   `json:"fingerprint"`
	SSID        string   `json:"ssid"` // saved home Wi-Fi, else the current one
	// WifiSuggestions are names to pick from: the current Wi-Fi first,
	// then saved and nearby networks.
	WifiSuggestions   []string `json:"wifiSuggestions"`
	Public            bool     `json:"public"`
	SetupURL          string   `json:"setupUrl"`
	SetupRemainingSec int      `json:"setupRemainingSec"`
}

// GetDeviceInfo describes how other devices can use the DNS server.
func (s *Service) GetDeviceInfo() DeviceInfo {
	st := s.x.Settings.Get()
	info := DeviceInfo{DNSAddrs: []string{}, DoHURLs: []string{}, SSID: st.DNSServer.IOSSSID, WifiSuggestions: []string{}}
	current := ""
	if s.x.CurrentSSID != nil {
		current = s.x.CurrentSSID()
	}
	if info.SSID == "" {
		info.SSID = current
	}
	if current != "" {
		info.WifiSuggestions = append(info.WifiSuggestions, current)
	}
	if s.x.WifiNames != nil {
		for _, n := range s.x.WifiNames() {
			if !slices.Contains(info.WifiSuggestions, n) {
				info.WifiSuggestions = append(info.WifiSuggestions, n)
			}
		}
	}
	if s.x.LANInfo != nil {
		info.Public = s.x.LANInfo().Public
	}
	if st.DNSServer.ShareLAN && s.o.d.LANAddrs != nil {
		for _, a := range s.o.d.LANAddrs() {
			info.DNSAddrs = append(info.DNSAddrs, a.String())
			info.DoHURLs = append(info.DoHURLs, dohURL(a, st.DNSServer.DoHPort))
		}
	}
	if ca := s.o.LANCA(); ca != nil {
		info.Fingerprint = ca.Fingerprint()
	}
	url, left := s.o.SetupInfo()
	info.SetupURL, info.SetupRemainingSec = url, int(left.Seconds())
	return info
}

func dohURL(a netip.Addr, port int) string {
	host := a.String()
	if a.Is6() {
		host = "[" + host + "]"
	}
	if port != 443 {
		host = fmt.Sprintf("%s:%d", host, port)
	}
	return "https://" + host + "/dns-query"
}

// SetDNSServer turns the DNS server on or off and sets the DoH port.
func (s *Service) SetDNSServer(enabled, shareLAN bool, dohPort int) error {
	st := s.x.Settings.Get()
	st.DNSServer.Enabled, st.DNSServer.ShareLAN, st.DNSServer.DoHPort = enabled, shareLAN, dohPort
	return s.saveSettings(st, true)
}

// SetIOSSSID saves the home Wi-Fi name used in the iOS profile.
func (s *Service) SetIOSSSID(ssid string) error {
	st := s.x.Settings.Get()
	st.DNSServer.IOSSSID = ssid
	return s.saveSettings(st, true)
}

// OpenSetupPage opens the phone setup page and returns its URL (for the QR
// code). The page closes by itself after ten minutes.
func (s *Service) OpenSetupPage() (string, error) {
	return s.o.OpenSetupPage(s.x.NewSetupPage, s.x.Settings.Get().DNSServer.IOSSSID)
}

// CloseSetupPage closes the phone setup page early.
func (s *Service) CloseSetupPage() error {
	s.o.CloseSetupPage()
	return nil
}

// SaveDeviceFiles saves the LAN CA (.crt) and, with a home Wi-Fi name,
// the iOS profile (.mobileconfig) through the native save dialog.
func (s *Service) SaveDeviceFiles(ctx context.Context) error {
	if s.x.SaveFile == nil || s.o.d.Certs == nil {
		return errors.New("saving files is not available")
	}
	ca := s.o.LANCA()
	if ca == nil {
		var err error
		if ca, err = s.o.d.Certs.LANCA(ctx); err != nil {
			return err
		}
	}
	st := s.x.Settings.Get().DNSServer
	var lan []netip.Addr
	if s.o.d.LANAddrs != nil {
		lan = s.o.d.LANAddrs()
	}
	files, err := lanFiles(ca, lan, st.DoHPort, st.IOSSSID)
	if err != nil {
		return err
	}
	if err := s.x.SaveFile(ctx, "ghostline-lan-ca.crt", files.CRT); err != nil {
		return err
	}
	if files.SSID == "" {
		return nil // no home Wi-Fi name: the certificate only
	}
	mc, err := files.MobileConfig(files.SSID)
	if err != nil {
		return err
	}
	return s.x.SaveFile(ctx, "ghostline.mobileconfig", mc)
}

// ResetLANCA replaces the LAN CA; other devices must install it again.
func (s *Service) ResetLANCA() error {
	if s.o.d.Certs == nil {
		return errors.New("certificates are not available")
	}
	if _, err := s.o.d.Certs.ResetLANCA(context.Background()); err != nil {
		return err
	}
	return s.o.ReapplyDNSServer(context.Background())
}

// RemoveLANCA turns the DNS server off and removes the LAN CA.
func (s *Service) RemoveLANCA() error {
	if s.o.d.Certs == nil {
		return errors.New("certificates are not available")
	}
	st := s.x.Settings.Get()
	if st.DNSServer.Enabled {
		st.DNSServer.Enabled = false
		if err := s.x.Settings.Save(st); err != nil {
			return err
		}
		if err := s.o.ReapplyDNSServer(context.Background()); err != nil {
			return err
		}
	}
	return s.o.d.Certs.RemoveLANCA(context.Background())
}

func dnsServerChanged(a, b store.DNSServerSettings) bool {
	return a.Enabled != b.Enabled || a.ShareLAN != b.ShareLAN || a.DoHPort != b.DoHPort
}
