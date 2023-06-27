package main

import (
	"net"
	"syscall"

	"github.com/miekg/dns"
)

func (r *Resolver) forward(query *dns.Msg) (*dns.Msg, error) {
	var record *syscall.DNSRecord
	if err := syscall.DnsQuery("google.com", dns.TypeAAAA, 0, nil, &record, nil); err != nil {
		return nil, err
	}
	msg := dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:            query.Id,
			Response:      true,
			Opcode:        0,
			Authoritative: false,
			Rcode:         dns.RcodeSuccess,
		},
		Question: query.Question,
		Answer: []dns.RR{
			&dns.AAAA{
				Hdr: dns.RR_Header{
					Name:     "google.com.",
					Rrtype:   dns.TypeAAAA,
					Class:    dns.ClassINET,
					Ttl:      record.Ttl,
					Rdlength: record.Length,
				},
				AAAA: net.IP(record.Data[:16]),
			},
		},
		Ns:    nil,
		Extra: nil,
	}
	return &msg, nil
}
