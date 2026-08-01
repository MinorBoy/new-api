package helper

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/seedancepricing"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func PreviewUserBillingQuota(c *gin.Context, input dto.CostPreviewRequest) (finalQuota int64, quotaPerUnitSnapshot string, err error) {
	return previewUserBillingQuotaForUser(c.GetInt("id"), input)
}

// previewUserBillingQuotaForUser is the userId-parameterized core of
// PreviewUserBillingQuota. Extracting it lets the routing revenue preview reuse the
// exact same pricing chain without fabricating a gin.Context just to carry a user id.
func previewUserBillingQuotaForUser(userId int, input dto.CostPreviewRequest) (finalQuota int64, quotaPerUnitSnapshot string, err error) {
	return previewUserBillingQuotaForUserWithSeedanceInput(userId, input, "", false, 0)
}

func previewUserBillingQuotaForUserWithSeedanceInput(userId int, input dto.CostPreviewRequest, resolution string, hasVideoInput bool, inputDurationMS int64) (finalQuota int64, quotaPerUnitSnapshot string, err error) {
	previewContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	requestPath := strings.TrimSpace(input.RequestPath)
	if requestPath == "" {
		requestPath = "/cost-accounting/preview"
	}
	previewContext.Request = httptest.NewRequest("POST", requestPath, nil)
	previewContext.Set("group", input.UserGroup)

	info := &relaycommon.RelayInfo{
		UserId:          userId,
		UserGroup:       input.UserGroup,
		UsingGroup:      input.UserGroup,
		StartTime:       time.Now(),
		OriginModelName: input.OriginModel,
		RelayMode:       input.RelayMode,
		RequestURLPath:  requestPath,
	}
	if input.ExpressionRequestInput != nil {
		body, marshalErr := common.Marshal(input.ExpressionRequestInput.Body)
		if marshalErr != nil {
			return 0, "", marshalErr
		}
		info.BillingRequestInput = &billingexpr.RequestInput{
			Headers: input.ExpressionRequestInput.Headers,
			Body:    body,
		}
	}

	if isCostPreviewPerCallMode(input.RelayMode) {
		priceData, priceErr := ModelPriceHelperPerCall(previewContext, info)
		if priceErr != nil {
			return 0, "", priceErr
		}
		if priceData.BillingMode == billing_setting.BillingModePerDuration && seedancepricing.Family(input.OriginModel) != "" && input.DurationSeconds != nil && *input.DurationSeconds > 0 {
			multiplier, ok := seedancepricing.DurationMultiplier(input.OriginModel, resolution, hasVideoInput, inputDurationMS, *input.DurationSeconds)
			if !ok {
				return 0, "", fmt.Errorf("unsupported Seedance duration pricing inputs for model %s", input.OriginModel)
			}
			ratios := priceData.OtherRatios()
			if ratios == nil {
				ratios = make(map[string]float64)
			}
			delete(ratios, "resolution")
			delete(ratios, "video_input")
			ratios["seedance_price_matrix"] = multiplier
			if !priceData.ReplaceOtherRatios(ratios) {
				return 0, "", fmt.Errorf("failed to apply Seedance duration pricing for model %s", input.OriginModel)
			}
		}
		info.PriceData = priceData
	} else {
		promptTokens := 0
		if input.Usage != nil {
			promptTokens = input.Usage.PromptTokens
		}
		meta := input.TokenMeta
		if meta == nil {
			meta = &relaytypes.TokenCountMeta{}
		}
		if _, err = ModelPriceHelper(previewContext, info, promptTokens, meta); err != nil {
			return 0, "", err
		}
	}

	quota, err := service.PreviewFinalUserQuota(previewContext, info, service.UserBillingPreviewInput{
		Usage:           input.Usage,
		DurationSeconds: input.DurationSeconds,
	})
	if err != nil {
		return 0, "", err
	}
	return quota, decimal.NewFromFloat(common.QuotaPerUnit).String(), nil
}

func isCostPreviewPerCallMode(relayMode int) bool {
	if relayMode == relayconstant.RelayModeSunoSubmit || relayMode == relayconstant.RelayModeVideoSubmit {
		return true
	}
	return relayMode >= relayconstant.RelayModeMidjourneyImagine && relayMode <= relayconstant.RelayModeMidjourneyEdits
}
