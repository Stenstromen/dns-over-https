package jsondns

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

func Marshal(msg *dns.Msg) *Response {
	now := time.Now().UTC()

	resp := new(Response)
	resp.Status = uint32(msg.Rcode)
	resp.TC = msg.Truncated
	resp.RD = msg.RecursionDesired
	resp.RA = msg.RecursionAvailable
	resp.AD = msg.AuthenticatedData
	resp.CD = msg.CheckingDisabled

	resp.Question = make([]Question, 0, len(msg.Question))
	for _, question := range msg.Question {
		jsonQuestion := Question{
			Name: question.Name,
			Type: question.Qtype,
		}
		resp.Question = append(resp.Question, jsonQuestion)
	}

	resp.Answer = make([]RR, 0, len(msg.Answer))
	for _, rr := range msg.Answer {
		jsonAnswer := marshalRR(rr, now)
		if !resp.HaveTTL || jsonAnswer.TTL < resp.LeastTTL {
			resp.HaveTTL = true
			resp.LeastTTL = jsonAnswer.TTL
			resp.EarliestExpires = jsonAnswer.Expires
		}
		resp.Answer = append(resp.Answer, jsonAnswer)
	}

	resp.Authority = make([]RR, 0, len(msg.Ns))
	for _, rr := range msg.Ns {
		jsonAuthority := marshalRR(rr, now)
		if !resp.HaveTTL || jsonAuthority.TTL < resp.LeastTTL {
			resp.HaveTTL = true
			resp.LeastTTL = jsonAuthority.TTL
			resp.EarliestExpires = jsonAuthority.Expires
		}
		resp.Authority = append(resp.Authority, jsonAuthority)
	}

	resp.Additional = make([]RR, 0, len(msg.Extra))
	for _, rr := range msg.Extra {
		jsonAdditional := marshalRR(rr, now)
		header := rr.Header()
		if header.Rrtype == dns.TypeOPT {
			opt := rr.(*dns.OPT)
			resp.Status = ((opt.Hdr.Ttl & 0xff000000) >> 20) | (resp.Status & 0xff)
			for _, option := range opt.Option {
				if option.Option() == dns.EDNS0SUBNET {
					edns0 := option.(*dns.EDNS0_SUBNET)
					clientAddress := edns0.Address
					if clientAddress == nil {
						clientAddress = net.IP{0, 0, 0, 0}
					} else if ipv4 := clientAddress.To4(); ipv4 != nil {
						clientAddress = ipv4
					}
					resp.EdnsClientSubnet = clientAddress.String() + "/" + strconv.FormatUint(uint64(edns0.SourceScope), 10)
				}
			}
			continue
		}
		if !resp.HaveTTL || jsonAdditional.TTL < resp.LeastTTL {
			resp.HaveTTL = true
			resp.LeastTTL = jsonAdditional.TTL
			resp.EarliestExpires = jsonAdditional.Expires
		}
		resp.Additional = append(resp.Additional, jsonAdditional)
	}

	return resp
}

func marshalRR(rr dns.RR, now time.Time) RR {
	jsonRR := RR{}
	rrHeader := rr.Header()
	jsonRR.Name = rrHeader.Name
	jsonRR.Type = rrHeader.Rrtype
	jsonRR.TTL = rrHeader.Ttl
	jsonRR.Expires = now.Add(time.Duration(jsonRR.TTL) * time.Second)
	jsonRR.ExpiresStr = jsonRR.Expires.Format(time.RFC1123)
	data := strings.SplitN(rr.String(), "\t", 5)
	if len(data) >= 5 {
		jsonRR.Data = data[4]
	}
	return jsonRR
}
