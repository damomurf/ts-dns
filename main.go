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
	A     net.IP
	AAAA  net.IP
	CNAME string
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
		if len(device.Tags) > 0 {
			for _, t := range device.Tags {
				fmt.Printf("Tag: %s\n", t)
				if strings.HasPrefix(t, "tag:"+*tagPrefix) {
					fmt.Printf("Adding Cname entry for %s\n", strings.TrimPrefix(t, "tag:"+*tagPrefix))
					tsDevices[strings.TrimPrefix(t, "tag:"+*tagPrefix)] = deviceEntry{
						CNAME: name,
					}
				}
			}
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

	rrTTL := uint32((*ttl).Seconds())

	switch r.Question[0].Qtype {
	case dns.TypeA:
		if tsDevices[host].CNAME != "" {
			// This is  CNAME, so return that record and the corresponding A
			rr := &dns.CNAME{
				Hdr:    dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: rrTTL},
				Target: tsDevices[host].CNAME + "." + *domain + ".",
			}
			m.Answer = append(m.Answer, rr)
			rrA := &dns.A{
				Hdr: dns.RR_Header{Name: tsDevices[host].CNAME + "." + *domain + ".", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: rrTTL},
				A:   tsDevices[tsDevices[host].CNAME].A.To4(),
			}
			m.Answer = append(m.Answer, rrA)
			break
		}
		rr := &dns.A{
			Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: rrTTL},
			A:   tsDevices[host].A.To4(),
		}
		m.Answer = append(m.Answer, rr)
	//m.Extra = append(m.Extra, t)
	case dns.TypeAAAA:
		if tsDevices[host].CNAME != "" {
			// This is  CNAME, so return that record and the corresponding AAAA
			rr := &dns.CNAME{
				Hdr:    dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: rrTTL},
				Target: tsDevices[host].CNAME + "." + *domain + ".",
			}
			m.Answer = append(m.Answer, rr)
			rrA := &dns.AAAA{
				Hdr:  dns.RR_Header{Name: tsDevices[host].CNAME + "." + *domain + ".", Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: rrTTL},
				AAAA: tsDevices[tsDevices[host].CNAME].AAAA.To16(),
			}
			m.Answer = append(m.Answer, rrA)
			break
		}
		rr := &dns.AAAA{
			Hdr:  dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: rrTTL},
			AAAA: tsDevices[host].AAAA.To16(),
		}
		m.Answer = append(m.Answer, rr)
	case dns.TypeCNAME:
		rr := &dns.CNAME{
			Hdr:    dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: rrTTL},
			Target: tsDevices[host].CNAME + "." + *domain + ".",
		}
		m.Answer = append(m.Answer, rr)
	}

	w.WriteMsg(m)
	log.Printf("%+v", m)
}

var (
	tailnet   = flag.String("tailnet", "murf.org", "The Tailscale tailnet name.")
	key       = flag.String("key", "", "Tailscale API Key")
	domain    = flag.String("domain", "murf.dev", "The domain to provide responses for.")
	interval  = flag.Duration("interval", 5*time.Minute, "How often to poll the Tailscale API for hosts.")
	tagPrefix = flag.String("tag-prefix", "cname-", "The tag prefix to use to generate CName entries against the hostname.")
	ttl       = flag.Duration("ttl", 60*time.Second, "The TTL to return on DNS entries.")
)

func main() {

	flag.Parse()

	err := fetchDevices(*tailnet, *key)
	if err != nil {
		log.Fatalf("%+v", err)
	}
	go func() {
		for _ = range time.Tick(*interval) {
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
