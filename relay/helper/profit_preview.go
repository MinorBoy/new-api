package helper

import (
	"github.com/QuantumNous/new-api/dto"
)

// PreviewRoutingRevenue is the routing-layer implementation of
// service.RoutingRevenuePreviewFunc. It reuses the same pricing chain
// (ModelPriceHelper + PreviewFinalUserQuota) as the live pre-consume charge and the
// cost-preview UI, so the predicted user revenue matches what the user would actually
// be billed.
//
// main.go installs this function via service.SetRoutingRevenuePreview so the service
// package never imports relay/helper (avoiding an import cycle). The callback only
// receives the client-facing origin model and the effective group: it never sees the
// channel's mapped upstream model, the supplier cost, or any reference video URL, so
// user revenue is computed exactly as a user would be billed.
func PreviewRoutingRevenue(originModelName, group, requestPath string, relayMode int, durationSeconds *int, userId int) (int64, string, error) {
	return PreviewRoutingRevenueWithSeedanceInput(originModelName, group, requestPath, relayMode, durationSeconds, userId, "", false, 0)
}

// PreviewRoutingRevenueWithSeedanceInput is the routing-layer preview with the
// request facts needed to reproduce Seedance's duration pricing. The extra facts
// stay in request memory and are never included in the public cost-preview DTO.
func PreviewRoutingRevenueWithSeedanceInput(originModelName, group, requestPath string, relayMode int, durationSeconds *int, userId int, resolution string, hasVideoInput bool, inputDurationMS int64) (int64, string, error) {
	input := dto.CostPreviewRequest{
		OriginModel:     originModelName,
		UserGroup:       group,
		RequestPath:     requestPath,
		RelayMode:       relayMode,
		DurationSeconds: durationSeconds,
	}
	return previewUserBillingQuotaForUserWithSeedanceInput(userId, input, resolution, hasVideoInput, inputDurationMS)
}
