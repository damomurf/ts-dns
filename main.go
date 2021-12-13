package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/pkg/errors"
)

type deviceEntry struct {
	A    net.IP
	AAAA net.IP
}

var tsDevices map[string]deviceEntry

func fetchDevices(tailnet, key string) error {

	client := &http.Client{
		Timeout: time.Second * 10,
	}

	req, err := http.NewRequest("GET", fmt.Sprintf(DeviceURL, tailnet), nil)
	if err != nil {
		return errors.Wrap(err, "creating request")
	}
	req.SetBasicAuth(key, "")

	response, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "executing request")
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return errors.Errorf("Tailscale API request returned unexpected status code: %d - %s", response.StatusCode, response.Status)
	}

	buf, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return errors.Wrap(err, "reading response body")
	}

	tnet := &Tailnet{}

	if err := json.Unmarshal(buf, tnet); err != nil {
		return errors.Wrap(err, "parsing JSON")
	}

	tsDevices = map[string]deviceEntry{}

	for _, device := range tnet.Devices {
		name := strings.Split(device.Name, ".")[0]
		tsDevices[name] = deviceEntry{
			A:    net.ParseIP(device.Addresses[0]),
			AAAA: net.ParseIP(device.Addresses[1]),
		}
	}

	log.Printf("%+v", tsDevices)

	return nil

}

func handleLookup(w dns.ResponseWriter, r *dns.Msg) {

	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	host := strings.Split(r.Question[0].Name, ".")[0]
	log.Printf("Received lookup for tailscale host: %s", host)
	log.Printf("%+v", r)

	_, ok := tsDevices[host]
	if ok == false {
		m.Rcode = dns.RcodeNameError
		w.WriteMsg(m)
		log.Printf("%+v", m)
		return
	}

	switch r.Question[0].Qtype {
	case dns.TypeA:
		rr := &dns.A{
			Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0},
			A:   tsDevices[host].A.To4(),
		}
		m.Answer = append(m.Answer, rr)
	//m.Extra = append(m.Extra, t)
	case dns.TypeAAAA:
		rr := &dns.AAAA{
			Hdr:  dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 0},
			AAAA: tsDevices[host].AAAA.To16(),
		}
		m.Answer = append(m.Answer, rr)
	}

	w.WriteMsg(m)
	log.Printf("%+v", m)
}

var (
	tailnet  = flag.String("tailnet", "murf.org", "The Tailscale tailnet name.")
	key      = flag.String("key", "", "Tailscale API Key")
	domain   = flag.String("domain", "murf.dev", "The domain to provide responses for.")
	interval = flag.Duration("interval", 5*time.Minute, "How often to poll the Tailscale API for hosts.")
)

func main() {

	flag.Parse()

	err := fetchDevices(*tailnet, *key)
	if err != nil {
		log.Fatalf("%+v", err)
	}
	go func() {
		for {
			time.Sleep(*interval)
			err := fetchDevices(*tailnet, *key)
			if err != nil {
				log.Printf("%+v", err)
			}
		}
	}()
	log.Println("Tailscale devices loaded...")
	dns.HandleFunc(fmt.Sprintf("%s.", *domain), handleLookup)
	server := &dns.Server{Addr: "[::]:8053", Net: "udp", TsigSecret: nil, ReusePort: true}
	if err := server.ListenAndServe(); err != nil {
		fmt.Printf("Failed to setup the udp server: %s\n", err.Error())
	}
}
