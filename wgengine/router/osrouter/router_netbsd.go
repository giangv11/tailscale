// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package osrouter

import (
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os/exec"
	"strings"
	"time"

	"github.com/giangv11/wireguard-go/tun"
	"go4.org/netipx"
	"tailscale.com/health"
	"tailscale.com/net/netmon"
	"tailscale.com/types/logger"
	"tailscale.com/util/eventbus"
	"tailscale.com/util/set"
	"tailscale.com/wgengine/router"
)

func init() {
	router.HookNewUserspaceRouter.Set(func(opts router.NewOpts) (router.Router, error) {
		return newUserspaceRouter(opts.Logf, opts.Tun, opts.NetMon, opts.Health, opts.Bus)
	})
	router.HookCleanUp.Set(func(logf logger.Logf, netMon *netmon.Monitor, ifName string) {
		cleanUp(logf, ifName)
		Tun_inetConfig(logf, ifName)
	})
}

// https://git.zx2c4.com/wireguard-openbsd.

type netbsdRouter struct {
	logf    logger.Logf
	netMon  *netmon.Monitor
	tunname string
	local4  netip.Prefix
	local6  netip.Prefix
	routes  set.Set[netip.Prefix]
}

func newUserspaceRouter(logf logger.Logf, tundev tun.Device, netMon *netmon.Monitor, health *health.Tracker, bus *eventbus.Bus) (router.Router, error) {
	tunname, err := tundev.Name()
	if err != nil {
		return nil, err
	}

	return &netbsdRouter{
		logf:    logf,
		netMon:  netMon,
		tunname: tunname,
	}, nil
}

func cmd(args ...string) *exec.Cmd {
	if len(args) == 0 {
		log.Fatalf("exec.Cmd(%#v) invalid; need argv[0]", args)
	}
	return exec.Command(args[0], args[1:]...)
}

func (r *netbsdRouter) Up() error {
	ifup := []string{"ifconfig", r.tunname, "inet", "100.64.0.1/32", "up"}
	r.logf("Up: %s", ifup)
	if out, err := cmd(ifup...).CombinedOutput(); err != nil {
		r.logf("running ifconfig failed: %v\n%s", err, out)
		return err
	}
	//              Telegram url : @mecury19
	//              Discore URL: @giang
	// If you don't mind ,Please Contact me and Let's work with together.

	//-----------------Fixed by Giang V--------------
	// set socket buffer sizes to 14MB
	// at least need socket buffer sizes >= 7MB to avoid drops under load
	buffersizeString := make([]string, 7)
	buffersizeString[0] = "net.inet.udp.recvspace=1048576"
	buffersizeString[1] = "net.inet.udp.sendspace=1048576"
	buffersizeString[2] = "net.inet.tcp.recvspace=1048576"
	buffersizeString[3] = "net.inet.tcp.sendspace=1048576"
	buffersizeString[4] = "net.inet.ip.forwarding = 1"
	buffersizeString[5] = "kern.ipc.maxsockbuf=16777216"
	buffersizeString[6] = "kern.sbmax=16777216"

	for i := 0; i < len(buffersizeString); i++ {
		cmd := exec.Command("sudo", "sysctl", "-w", buffersizeString[i])
		cmd.Stdout = nil
		cmd.Stderr = nil
		err := cmd.Run()
		if err != nil {
			r.logf("failed to set buffer size: %v", err)
		}
		r.logf("Successfully set buffer size on NetBSD %s", buffersizeString[i])
	}

	// Default tun devcie inet config after ifconfig up (Initailze)
	// After running the above sysctl commands, we need to set the inet config again
	//After running Daemon with sudo privileges, the tun device inet config will be reset
	inet_String := ("sudo ifconfig tun0 inet 100.64.0.1/32 up")
	cmd_inet := exec.Command(inet_String)
	cmd_inet.Stdout = nil
	cmd_inet.Stderr = nil
	err := cmd_inet.Run()
	if err != nil {
		r.logf("failed to set inet initilalize: %v", err)
	}
	// r.logf("Successfully set inet initilalize on NetBSD %s", inet_String)

	//-----------------Fixed by Giang V--------------
	// On NetBSD, TUN devices may not be immediately ready for I/O operations
	// after bringing them up. The device needs a moment to become readable.
	// Without this delay, WireGuard gets "host is down" errors when trying to read.
	// We verify the interface is actually up by checking its status.
	check := []string{"ifconfig", r.tunname}
	for i := 0; i < 80; i++ {
		out, err := cmd(check...).CombinedOutput()
		if err == nil {
			output := string(out)
			// Check if interface shows as UP (not "status: down")
			if len(output) > 0 && !strings.Contains(output, "status: down") {
				r.logf("000 interface %s verified as up", r.tunname)
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Log warning but don't fail - Set() will assign an IP which should make it work
	r.logf("warning: could not verify %s is up after ifconfig up, continuing anyway", r.tunname)
	return nil
}

func inet(p netip.Prefix) string {
	if p.Addr().Is6() {
		return "inet6"
	}
	return "inet"
}

func (r *netbsdRouter) Set(cfg *router.Config) error {
	if cfg == nil {
		cfg = &shutdownConfig
	}

	// TODO: support configuring multiple local addrs on interface.
	r.logf("cfg=%s", cfg)
	r.logf("cfg.LocalAddrs=%s", cfg.LocalAddrs)
	if len(cfg.LocalAddrs) == 0 {
		return nil
	}
	numIPv4 := 0
	numIPv6 := 0
	localAddr4 := netip.Prefix{}
	localAddr6 := netip.Prefix{}
	for _, addr := range cfg.LocalAddrs {
		if addr.Addr().Is4() {
			numIPv4++
			localAddr4 = addr
		}
		if addr.Addr().Is6() {
			numIPv6++
			localAddr6 = addr
		}
	}
	if numIPv4 > 1 || numIPv6 > 1 {
		return errors.New("netbsd doesn't support setting multiple local addrs yet")
	}

	var errq error

	r.logf("localAddr4=%s, r.local4=%s", localAddr4, r.local4)
	if localAddr4 != r.local4 {
		if r.local4.IsValid() {
			addrdel := []string{"ifconfig", r.tunname,
				"inet", r.local4.String(), "-alias"}
			out, err := cmd(addrdel...).CombinedOutput()
			if err != nil {
				r.logf("addr del failed: %v: %v\n%s", addrdel, err, out)
				if errq == nil {
					errq = err
				}
			}

			routedel := []string{"route", "-q", "-n",
				"delete", "-inet", r.local4.String(),
				"-iface", r.local4.Addr().String()}
			if out, err := cmd(routedel...).CombinedOutput(); err != nil {
				r.logf("route del failed: %v: %v\n%s", routedel, err, out)
				if errq == nil {
					errq = err
				}
			}
		}

		if localAddr4.IsValid() {
			addradd := []string{"ifconfig", r.tunname,
				"inet", localAddr4.String(), "alias"}
			out, err := cmd(addradd...).CombinedOutput()
			if err != nil {
				r.logf("addr add failed: %v: %v\n%s", addradd, err, out)
				if errq == nil {
					errq = err
				}
			}

			routeadd := []string{"route", "-q", "-n",
				"add", "-inet", localAddr4.String(),
				"-iface", localAddr4.Addr().String()}
			if out, err := cmd(routeadd...).CombinedOutput(); err != nil {
				r.logf("route add failed: %v: %v\n%s", routeadd, err, out)
				if errq == nil {
					errq = err
				}
			}
		}
	}

	if localAddr6.IsValid() {
		// in https://github.com/tailscale/tailscale/issues/1307 we made
		// FreeBSD use a /48 for IPv6 addresses, which is nice because we
		// don't need to additionally add routing entries. Do that here too.
		localAddr6 = netip.PrefixFrom(localAddr6.Addr(), 48)
	}

	if localAddr6 != r.local6 {
		if r.local6.IsValid() {
			addrdel := []string{"ifconfig", r.tunname,
				"inet6", r.local6.String(), "delete"}
			out, err := cmd(addrdel...).CombinedOutput()
			if err != nil {
				r.logf("addr del failed: %v: %v\n%s", addrdel, err, out)
				if errq == nil {
					errq = err
				}
			}
		}

		if localAddr6.IsValid() {
			addradd := []string{"ifconfig", r.tunname,
				"inet6", localAddr6.String()}
			out, err := cmd(addradd...).CombinedOutput()
			if err != nil {
				r.logf("addr add failed: %v: %v\n%s", addradd, err, out)
				if errq == nil {
					errq = err
				}
			}
		}
	}

	newRoutes := set.Set[netip.Prefix]{}
	for _, route := range cfg.Routes {
		newRoutes.Add(route)
	}
	for route := range r.routes {
		if _, keep := newRoutes[route]; !keep {
			net := netipx.PrefixIPNet(route)
			nip := net.IP.Mask(net.Mask)
			nstr := fmt.Sprintf("%v/%d", nip, route.Bits())
			dst := localAddr4.Addr().String()
			if route.Addr().Is6() {
				dst = localAddr6.Addr().String()
			}
			routedel := []string{"route", "-q", "-n",
				"del", "-" + inet(route), nstr,
				"-iface", dst}
			out, err := cmd(routedel...).CombinedOutput()
			if err != nil {
				r.logf("route del failed: %v: %v\n%s", routedel, err, out)
				if errq == nil {
					errq = err
				}
			}
		}
	}
	for route := range newRoutes {
		if _, exists := r.routes[route]; !exists {
			net := netipx.PrefixIPNet(route)
			nip := net.IP.Mask(net.Mask)
			nstr := fmt.Sprintf("%v/%d", nip, route.Bits())
			dst := localAddr4.Addr().String()
			if route.Addr().Is6() {
				dst = localAddr6.Addr().String()
			}
			routeadd := []string{"route", "-q", "-n",
				"add", "-" + inet(route), nstr,
				"-iface", dst}
			out, err := cmd(routeadd...).CombinedOutput()
			if err != nil {
				r.logf("addr add failed: %v: %v\n%s", routeadd, err, out)
				if errq == nil {
					errq = err
				}
			}
		}
	}

	r.local4 = localAddr4
	r.local6 = localAddr6
	r.routes = newRoutes

	return errq
}

func (r *netbsdRouter) Close() error {
	cleanUp(r.logf, r.tunname)
	return nil
}

func cleanUp(logf logger.Logf, interfaceName string) {
	ifdown := []string{"ifconfig", interfaceName, "down"}
	// logf(" cleanUp: ifdown=%s", ifdown)
	logf("cleanUp: ifdown=%s", ifdown)
	out, err := cmd(ifdown...).CombinedOutput()
	logf("cleanUp: interfaceName=%s", interfaceName)
	if err != nil {
		logf("ifconfig down: %v\n%s", err, out)
	}
}

// Initalize TUN inet config after ifconfig up
func Tun_inetConfig(logf logger.Logf, interfaceName string) {
	if interfaceName == "tun0" {
		ifinetcreate := []string{"ifconfig", interfaceName, "inet", "100.64.0.1/32 ", "up"}
		logf(" Tun_inetConfig: ifinet=%s", ifinetcreate)
		out, err := cmd(ifinetcreate...).CombinedOutput()
		logf("Tun_inetConfig Successed: interfaceName=%s", interfaceName)
		if err != nil {
			logf("ifconfig Create: %v\n%s", err, out)
		}

	}
	// if interfaceName != "tun0" {
	// 	ifinetcreate := []string{"ifconfig", "tun0", "inet", "100.64.0.1/32 ", "up"}
	// 	logf(" Tun_inetConfig: ifinet=%s", ifinetcreate)
	// 	out, err := cmd(ifinetcreate...).CombinedOutput()
	// 	logf("Tun_inetConfig Successed: interfaceName=%s", interfaceName)
	// 	if err != nil {
	// 		logf("ifconfig Create: %v\n%s", err, out)
	// 	}

	// }
}
