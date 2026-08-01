package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
)

var publicDoubaoVendor = model.PricingVendor{
	ID: -1, Name: "Doubao", Icon: "Doubao.Color",
}

type publicPricingProjection struct {
	Pricing            []model.Pricing
	Vendors            []model.PricingVendor
	SupportedEndpoints map[string]common.EndpointInfo
}

func projectPublicPricing(pricing []model.Pricing, endpoints map[string]common.EndpointInfo) publicPricingProjection {
	pricingByName := make(map[string]model.Pricing, len(pricing))
	for _, item := range pricing {
		pricingByName[item.ModelName] = item
	}

	projection := publicPricingProjection{
		Pricing:            make([]model.Pricing, 0, len(modelrouting.CanonicalModels)),
		Vendors:            []model.PricingVendor{},
		SupportedEndpoints: make(map[string]common.EndpointInfo),
	}
	for _, modelName := range modelrouting.CanonicalModels {
		item, ok := pricingByName[modelName]
		if !ok {
			continue
		}
		item.VendorID = publicDoubaoVendor.ID
		item.Icon = publicDoubaoVendor.Icon
		item.OwnerBy = modelrouting.PublicModelOwner
		projection.Pricing = append(projection.Pricing, item)
		for _, endpointType := range item.SupportedEndpointTypes {
			key := string(endpointType)
			if endpoint, exists := endpoints[key]; exists {
				projection.SupportedEndpoints[key] = endpoint
			}
		}
	}
	if len(projection.Pricing) > 0 {
		projection.Vendors = append(projection.Vendors, publicDoubaoVendor)
	}
	return projection
}
