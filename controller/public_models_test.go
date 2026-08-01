package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectPublicPricingRemovesInternalModelsAndVendors(t *testing.T) {
	pricing := []model.Pricing{
		{ModelName: "provider-hidden", VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIResponse}},
		{ModelName: modelrouting.Seedance20Mini, VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
		{ModelName: modelrouting.Seedance20, VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
		{ModelName: modelrouting.Seedance20Fast, VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
	}
	endpoints := map[string]common.EndpointInfo{
		string(constant.EndpointTypeOpenAI):         {Path: "/v1/chat/completions", Method: "POST"},
		string(constant.EndpointTypeOpenAIResponse): {Path: "/v1/responses", Method: "POST"},
	}

	projection := projectPublicPricing(pricing, endpoints)
	require.Equal(t, []string{
		modelrouting.Seedance20, modelrouting.Seedance20Fast, modelrouting.Seedance20Mini,
	}, []string{
		projection.Pricing[0].ModelName,
		projection.Pricing[1].ModelName,
		projection.Pricing[2].ModelName,
	})
	for _, item := range projection.Pricing {
		assert.Equal(t, publicDoubaoVendor.ID, item.VendorID)
		assert.Equal(t, modelrouting.PublicModelOwner, item.OwnerBy)
		assert.Equal(t, publicDoubaoVendor.Icon, item.Icon)
	}
	require.Equal(t, []model.PricingVendor{publicDoubaoVendor}, projection.Vendors)
	require.Contains(t, projection.SupportedEndpoints, string(constant.EndpointTypeOpenAI))
	require.NotContains(t, projection.SupportedEndpoints, string(constant.EndpointTypeOpenAIResponse))
}
