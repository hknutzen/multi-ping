/*
ping-multi -- Ping many IP addresses rapidly.

Coprigtht (C) 2024 Heinz Knutzen <heinz.knutzen@googlemail.com>

This program is free software; you can redistribute it and/or
modify it under the terms of the GNU General Public License
as published by the Free Software Foundation; either version 2
of the License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program; if not, write to the Free Software
Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301, USA.
*/

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"

	"go4.org/netipx"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

var debug = flag.Bool("D", false, "Show debug messages")

func main() {
	delay := flag.Duration("d", time.Millisecond*100, "Delay between successive pings")
	timeout := flag.Duration("t", time.Second, "Timeout for response")
	showUnreachable := flag.Bool("u", false, "Show only unreachable addresses")
	showReachable := flag.Bool("r", false, "Show only reachable addresses")
	flag.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			"Usage:\n  %s [options] [filename-with-IP-addresses]\n\nOptions:\n",
			os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if !*showUnreachable && !*showReachable {
		*showUnreachable = true
		*showReachable = true
	}

	log.SetFlags(0) // Don't log date & time
	var data []byte
	var err error
	switch flag.NArg() {
	case 0:
		data, err = ioutil.ReadAll(os.Stdin)
	case 1:
		data, err = ioutil.ReadFile(flag.Arg(0))
	default:
		flag.Usage()
		return
	}
	if err != nil {
		log.Fatal(err)
	}

	// List of IP addresses to be pinged.
	list := strings.Split(string(data), "\n")
	ipList := make([]netip.Addr, 0)
	var hasIPv4, hasIPv6 bool

	// Remove leading and trailing white space and empty lines.
	// Log invalid IP addresses.
	for _, line := range list {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rg netipx.IPRange
		if ip, err := netip.ParseAddr(line); err == nil {
			rg = netipx.IPRangeFrom(ip, ip)
		} else if ipp, err := netip.ParsePrefix(line); err == nil {
			rg = netipx.RangeOfPrefix(ipp)
		} else if ipr, err := netipx.ParseIPRange(line); err == nil {
			rg = ipr
		} else {
			log.Printf("Ignoring invalid IP '%s'", line)
			continue
		}
		for ip := rg.From(); ip != rg.To(); ip = ip.Next() {
			ipList = append(ipList, ip)
		}
		if rg.From().Is4() {
			hasIPv4 = true
		} else {
			hasIPv6 = true
		}
	}

	if len(ipList) == 0 {
		return
	}

	// Remember full list for showing also reachable addresses in result.
	fullList := ipList

	// Remember time when each ping to address was sent.
	// Entries will be deleted, if echo reply is received in time.
	// So in the end this holds only unreplied addresses.
	ipSent := make(map[string]time.Time, len(ipList))

	recv := make(chan string, 5)
	var conn4, conn6 *icmp.PacketConn
	if hasIPv4 {
		conn4 = createConn("udp4")
		go recvICMP(conn4, recv)
	}
	if hasIPv6 {
		conn6 = createConn("udp6")
		go recvICMP(conn6, recv)
	}

	// Send packets with this delay.
	ticker := time.NewTicker(*delay)

	// Timer will be restarted after last ping has been sent.
	wait := time.NewTimer(*timeout)
	wait.Stop()

	nextPing := func(time time.Time) {
		ip := ipList[0]
		ipList = ipList[1:]
		if len(ipList) == 0 {
			ticker.Stop()
			wait.Reset(*timeout)
		}
		debugf("Sending: %s", ip.String())
		sendICMP(conn4, conn6, ip)
		ipSent[ip.String()] = time
	}

	nextPing(time.Now())
LOOP:
	for {
		select {

		case time := <-ticker.C:
			if len(ipList) == 0 {
				debugf("Ignoring excess ticker event")
				continue
			}
			nextPing(time)

		case addr := <-recv:
			// Receive echo reply
			debugf("Received: %s", addr)
			if start, found := ipSent[addr]; found {
				if *timeout >= time.Now().Sub(start) {

					// Remove entries with reply.
					delete(ipSent, addr)
				}
			}

		case <-wait.C:
			// Wait after last echo has been sent.
			debugf("Timeout finished")
			break LOOP
		}
	}

	// Print result.
	for _, ip := range fullList {
		addr := ip.String()
		_, unreachable := ipSent[addr]
		if *showUnreachable && *showReachable {
			what := "ok"
			if unreachable {
				what = "failed"
			}
			fmt.Printf("%s\t%s\n", addr, what)
		} else if *showUnreachable && unreachable ||
			*showReachable && !unreachable {
			fmt.Println(addr)
		}
	}
}

func createConn(typ string) *icmp.PacketConn {
	conn, err := icmp.ListenPacket(typ, "")
	if err != nil {
		log.Fatalf("%v\nCheck /proc/sys/net/ipv4/ping_group_range", err)
	}
	return conn
}

func recvICMP(conn *icmp.PacketConn, recv chan<- string) {
	bytes := make([]byte, 512)
	for {
		_, remoteAddr, err := conn.ReadFrom(bytes)
		if err != nil {
			panic(err)
		}
		if addr, ok := remoteAddr.(*net.UDPAddr); ok {
			recv <- addr.IP.String()
		} else {
			panic(remoteAddr)
		}
	}
}

func sendICMP(conn4, conn6 *icmp.PacketConn, ip netip.Addr) {
	dst := &net.UDPAddr{IP: ip.AsSlice()}
	var typ icmp.Type
	var conn *icmp.PacketConn
	if ip.Is4() {
		typ = ipv4.ICMPTypeEcho
		conn = conn4
	} else {
		typ = ipv6.ICMPTypeEchoRequest
		conn = conn6
	}
	bytes, err := (&icmp.Message{
		Type: typ, Code: 0,
		Body: &icmp.Echo{
			ID:  rand.Intn(65535),
			Seq: 1,
		},
	}).Marshal(nil)
	if err != nil {
		panic(err)
	}

	// Ignore errors, destination will simply be unreachable.
	_, _ = conn.WriteTo(bytes, dst)
}

func debugf(f string, v ...interface{}) {
	if *debug {
		log.Printf(f, v...)
	}
}
