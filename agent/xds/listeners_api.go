package xds

import (
	"errors"

	"google.golang.org/protobuf/proto"

	"github.com/hashicorp/consul/agent/proxycfg"
)

func (s *ResourceGenerator) makeAPIGatewayListeners(address string, cfgSnap *proxycfg.ConfigSnapshot) ([]proto.Message, error) {
	return nil, errors.New("implement me")
}
