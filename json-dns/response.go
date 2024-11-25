package jsondns

import (
	"encoding/json"
	"time"
)

type QuestionList []Question

func (ql *QuestionList) UnmarshalJSON(b []byte) error {
	// Fix variant question response in Response.Question
	//
	// Solution taken from:
	//	https://engineering.bitnami.com/articles/dealing-with-json-with-non-homogeneous-types-in-go.html
	//	https://archive.is/NU4zR
	if len(b) > 0 && b[0] == '[' {
		return json.Unmarshal(b, (*[]Question)(ql))
	}
	var q Question
	if err := json.Unmarshal(b, &q); err != nil {
		return err
	}
	*ql = []Question{q}
	return nil
}

type Response struct {
	// Standard DNS response code (32 bit integer)
	Status uint32 `json:"Status"`
	// Whether the response is truncated
	TC bool `json:"TC"`
	// Recursion desired
	RD bool `json:"RD"`
	// Recursion available
	RA bool `json:"RA"`
	// Whether all response data was validated with DNSSEC
	// FIXME: We don't have DNSSEC yet! This bit is not reliable!
	AD bool `json:"AD"`
	// Whether the client asked to disable DNSSEC
	CD               bool         `json:"CD"`
	Question         QuestionList `json:"Question"`
	Answer           []RR         `json:"Answer,omitempty"`
	Authority        []RR         `json:"Authority,omitempty"`
	Additional       []RR         `json:"Additional,omitempty"`
	Comment          string       `json:"Comment,omitempty"`
	EdnsClientSubnet string       `json:"edns_client_subnet,omitempty"`
	// Least time-to-live
	HaveTTL         bool      `json:"-"`
	LeastTTL        uint32    `json:"-"`
	EarliestExpires time.Time `json:"-"`
}

type Question struct {
	// FQDN with trailing dot
	Name string `json:"name"`
	// Standard DNS RR type
	Type uint16 `json:"type"`
}

type RR struct {
	Question
	// Record's time-to-live in seconds
	TTL uint32 `json:"TTL"`
	// TTL in absolute time
	Expires    time.Time `json:"-"`
	ExpiresStr string    `json:"Expires"`
	// Data
	Data string `json:"data"`
}
