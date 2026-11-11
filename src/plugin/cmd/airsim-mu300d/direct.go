package main

import (
	"errors"
	"net"
	"net/http"
	"strconv"
)

func tailnetIPv4(ip net.IP) bool {
	v := ip.To4()
	return v != nil && v[0] == 100 && v[1] >= 64 && v[1] <= 127
}
func privateIPv4(ip net.IP) bool {
	return ip.To4() != nil && (ip.IsPrivate() || ip.IsLinkLocalUnicast() || tailnetIPv4(ip))
}

// directInterfaceNames 是允许承载直连监听的网卡白名单：LAN 桥、无线网卡、可选隧道。
// 蜂窝口（sipa_eth0 之类）永远不在白名单里 —— 运营商常给 10.x 私有地址，
// 无脑"枚举所有私有 IPv4"会把 8580-8582 绑到运营商内网，等于对外开洞。
func directInterfaceNames(s managementSettings) []string {
	names := []string{s.Interface}
	if s.STAInterface != "" && s.STAInterface != s.Interface {
		names = append(names, s.STAInterface)
	}
	if s.TunnelInterface != "" && s.TunnelInterface != s.Interface {
		names = append(names, s.TunnelInterface)
	}
	return names
}

// interfaceAddress 取某网卡上第一个私有 IPv4；tunnelOnly 时只认 100.64/10。
func interfaceAddress(name string, tunnelOnly bool) (string, bool) {
	i, err := net.InterfaceByName(name)
	if err != nil || i.Flags&net.FlagUp == 0 {
		return "", false
	}
	addrs, err := i.Addrs()
	if err != nil {
		return "", false
	}
	for _, a := range addrs {
		ip, _, err := net.ParseCIDR(a.String())
		if err != nil || ip.To4() == nil {
			continue
		}
		if tunnelOnly {
			if !tailnetIPv4(ip) {
				continue
			}
		} else if !privateIPv4(ip) {
			continue
		}
		return ip.String(), true
	}
	return "", false
}

// directAddresses 列出所有要监听的地址。主网卡（LAN 桥）必须有地址，缺了就是硬错；
// 无线与隧道按"当前有没有地址"尽力而为 —— 热点模式下无线没地址、Wi-Fi 模式下
// 隧道没连上，都不该让服务起不来。
func directAddresses(s managementSettings) ([]string, error) {
	primary, ok := interfaceAddress(s.Interface, false)
	if !ok {
		return nil, errors.New("LAN 网卡没有私有 IPv4 地址")
	}
	hosts := []string{primary}
	if s.STAInterface != "" {
		if ip, ok := interfaceAddress(s.STAInterface, false); ok && ip != primary {
			hosts = append(hosts, ip)
		}
	}
	if s.TunnelInterface != "" {
		if ip, ok := interfaceAddress(s.TunnelInterface, true); ok && ip != primary {
			hosts = append(hosts, ip)
		}
	}
	return hosts, nil
}

type directEndpoint struct{ pair, control, pcm net.Listener }
type directListeners struct {
	endpoints []directEndpoint
	all       []net.Listener
}

func (d *directListeners) close() {
	for _, l := range d.all {
		l.Close()
	}
	d.all = nil
	d.endpoints = nil
}

func (d *directListeners) hostCount() int { return len(d.endpoints) }

// bindDirectListeners 在尽量多的地址上开监听：某个地址暂时不可用（地址刚消失、
// 端口被占）时跳过它，而不是让整个服务起不来。一个都绑不上才算失败 —— 那时
// 签名响应也确实没法给出可用的网关端口。
func bindDirectListeners(hosts []string, s managementSettings) (*directListeners, error) {
	d := &directListeners{}
	var firstErr error
	for _, host := range hosts {
		var local []net.Listener
		ok := true
		for _, port := range []int{s.PairPort, s.ControlPort, s.PCMPort} {
			l, err := net.Listen("tcp4", net.JoinHostPort(host, strconv.Itoa(port)))
			if err != nil {
				firstErr = err
				ok = false
				break
			}
			local = append(local, l)
		}
		if !ok {
			for _, l := range local {
				l.Close()
			}
			continue
		}
		d.all = append(d.all, local...)
		d.endpoints = append(d.endpoints, directEndpoint{local[0], local[1], local[2]})
	}
	if d.hostCount() == 0 {
		d.close()
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, errors.New("没有可用的直连监听地址")
	}
	return d, nil
}

func (d *daemon) hasDirectRoute(r *http.Request) bool {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}
	d.listenersMu.Lock()
	direct := d.directHosts[host]
	d.listenersMu.Unlock()
	return direct
}
func privatePairing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !privateIPv4(net.ParseIP(host)) {
			write(w, http.StatusForbidden, map[string]string{"error": "private_network_required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
