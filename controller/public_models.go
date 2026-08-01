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

func projectPublicPricing(
	pricing []model.Pricing,
	vendors []model.PricingVendor,
	endpoints map[string]common.EndpointInfo,
) publicPricingProjection {
	projection := publicPricingProjection{
		Pricing:            make([]model.Pricing, 0, len(pricing)),
		Vendors:            []model.PricingVendor{},
		SupportedEndpoints: make(map[string]common.EndpointInfo),
	}
	usedVendorIDs := make(map[int]struct{})
	includeDoubaoVendor := false
	for _, item := range pricing {
		if modelrouting.IsHiddenSeedanceModel(item.ModelName) {
			continue
		}
		if modelrouting.IsPublicSeedanceModel(item.ModelName) {
			item.VendorID = publicDoubaoVendor.ID
			item.Icon = publicDoubaoVendor.Icon
			item.OwnerBy = modelrouting.PublicModelOwner
			includeDoubaoVendor = true
		} else {
			usedVendorIDs[item.VendorID] = struct{}{}
		}
		projection.Pricing = append(projection.Pricing, item)
		for _, endpointType := range item.SupportedEndpointTypes {
			key := string(endpointType)
			if endpoint, exists := endpoints[key]; exists {
				projection.SupportedEndpoints[key] = endpoint
			}
		}
	}
	for _, vendor := range vendors {
		if _, ok := usedVendorIDs[vendor.ID]; ok {
			projection.Vendors = append(projection.Vendors, vendor)
		}
	}
	if includeDoubaoVendor {
		projection.Vendors = append(projection.Vendors, publicDoubaoVendor)
	}
	return projection
}
