// Package piiktv hands Piik room links to the Piik TV app (it.mbs.piiktv).
//
// The TV app runs a small HTTP receiver on TCP 8790 while it is open:
// GET /info answers {"app":"piik-tv","name":...} and POST /link (form field
// "link") opens that room on the TV. The App on this computer finds the TVs on
// its own private IPv4 LANs and delivers the link itself, so a page never needs
// a TV address or a cross-origin request into the LAN.
package piiktv

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// Port is the Piik TV link receiver.
	Port = 8790
	// 128 probes at a time keep a /22 (1022 addresses) within about 2.5 s.
	scanWorkers = 128
	// SendTimeout bounds one discovery and delivery round.
	SendTimeout = 6 * time.Second
)

// linkPattern is a Piik room link: scheme, host, optional port and /r/<code>,
// with any query or #v= viewer grant. The TV app accepts exactly this shape.
var linkPattern = regexp.MustCompile(`^https?://[A-Za-z0-9.\-]+(?::\d{1,5})?/r/[1-9][0-9]{3}[^\s]*$`)

var client = &http.Client{Timeout: 1500 * time.Millisecond}

// Target is one Piik TV app found on the LAN.
type Target struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

// ValidLink reports whether link is a Piik room link the TV app can open.
func ValidLink(link string) bool {
	return len(link) <= 1024 && linkPattern.MatchString(link)
}

// SendAll finds every open Piik TV app and sends it link. It returns the TVs
// that accepted the link; none means no TV app is open on these networks.
func SendAll(ctx context.Context, link string) []Target {
	sent := []Target{}
	for _, tv := range Discover(ctx) {
		if Send(ctx, tv.Address, link) == nil {
			sent = append(sent, tv)
		}
	}
	return sent
}

// candidates lists the other addresses of this computer's private IPv4 networks.
func candidates() []string {
	seen := map[string]bool{}
	var list []string
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			network, ok := address.(*net.IPNet)
			if !ok {
				continue
			}
			own := network.IP.To4()
			if own == nil || !own.IsPrivate() {
				continue
			}
			for _, candidate := range subnetHosts(own, network.Mask) {
				if !seen[candidate] {
					seen[candidate] = true
					list = append(list, candidate)
				}
			}
		}
	}
	return list
}

// subnetHosts lists the other host addresses of own's network, nearest first:
// the whole subnet up to a /22 (1022 hosts, as some mesh routers use), only the
// /24 around own for larger networks, and the /24 when the mask is unknown.
func subnetHosts(own net.IP, mask net.IPMask) []string {
	own = own.To4()
	if own == nil {
		return nil
	}
	ones, bits := mask.Size()
	if bits != 32 || ones < 22 {
		ones = 24
	}
	if ones > 30 {
		return nil // /31 and /32 have no other hosts to reach
	}
	base := binary.BigEndian.Uint32(own.Mask(net.CIDRMask(ones, 32)))
	self := binary.BigEndian.Uint32(own)
	size := uint32(1) << (32 - ones)
	// Own /24 first, so the usual home network is probed before the rest of a larger one.
	ordered := make([]uint32, 0, size)
	block := self &^ 0xff
	for value := base + 1; value < base+size-1; value++ {
		if value&^0xff == block {
			ordered = append(ordered, value)
		}
	}
	for value := base + 1; value < base+size-1; value++ {
		if value&^0xff != block {
			ordered = append(ordered, value)
		}
	}
	hosts := make([]string, 0, len(ordered))
	for _, value := range ordered {
		if value == self {
			continue
		}
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, value)
		hosts = append(hosts, ip.String())
	}
	return hosts
}

// Discover probes the LANs for open Piik TV apps.
func Discover(ctx context.Context) []Target {
	addresses := candidates()
	jobs := make(chan string)
	results := make(chan Target, len(addresses))
	var workers sync.WaitGroup
	for range scanWorkers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for address := range jobs {
				if tv, ok := probe(ctx, address); ok {
					results <- tv
				}
			}
		}()
	}
feed:
	for _, address := range addresses {
		select {
		case jobs <- address:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	workers.Wait()
	close(results)
	var tvs []Target
	for tv := range results {
		tvs = append(tvs, tv)
	}
	return tvs
}

func probe(ctx context.Context, address string) (Target, bool) {
	dialer := net.Dialer{Timeout: 300 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(address, strconv.Itoa(Port)))
	if err != nil {
		return Target{}, false
	}
	_ = conn.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://%s:%d/info", address, Port), nil)
	if err != nil {
		return Target{}, false
	}
	response, err := client.Do(request)
	if err != nil {
		return Target{}, false
	}
	defer response.Body.Close()
	var info struct {
		App  string `json:"app"`
		Name string `json:"name"`
	}
	if response.StatusCode != http.StatusOK ||
		json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&info) != nil || info.App != "piik-tv" {
		return Target{}, false
	}
	return Target{Address: address, Name: info.Name}, true
}

// Send hands link to the Piik TV app at address.
func Send(ctx context.Context, address, link string) error {
	form := url.Values{"link": {link}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("http://%s:%d/link", address, Port), strings.NewReader(form))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 65536))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Piik TV answered %d", response.StatusCode)
	}
	return nil
}
