package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
)

// RouteTargetContractValidator is registered by relay at process startup to
// keep provider protocol knowledge out of the service package.
var RouteTargetContractValidator func(channel *model.Channel, canonicalModel string, target modelrouting.Target) error
