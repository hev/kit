// Package hostrelay defines the wire format used by outbound execution hosts.
package hostrelay

import (
	"github.com/hev/kit/pkg/orders/v1"
	"github.com/hev/kit/pkg/peers/v1"
)

const Path = "/v1/host-relay"

type Job struct {
	ID    string          `json:"id"`
	Order *orders.Request `json:"order,omitempty"`
	Peer  *peers.Request  `json:"peer,omitempty"`
}

type Result struct {
	ID    string           `json:"id"`
	Order *orders.RPCReply `json:"order,omitempty"`
	Peer  *peers.Reply     `json:"peer,omitempty"`
}
