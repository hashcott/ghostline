package sysproxy

import "strings"

// localNets are kept off the proxy on desktops that understand CIDR.
var localNets = []string{"localhost", "127.0.0.0/8", "::1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}

// BypassWinINET is the WinINET bypass list: WinINET has no CIDR, so the
// 172.16/12 range is spelled out (unchanged since v0.5).
func BypassWinINET() string {
	return "<local>;localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;[::1]"
}

// BypassGNOME is org.gnome.system.proxy ignore-hosts, as gsettings text.
func BypassGNOME() string { return "['" + strings.Join(localNets, "', '") + "']" }

// BypassKDE is kioslaverc NoProxyFor.
func BypassKDE() string { return strings.Join(localNets, ",") }
