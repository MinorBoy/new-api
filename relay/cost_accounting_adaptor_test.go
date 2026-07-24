package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfirmCostIdentityUsesFinalOverriddenModel(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "mapped-model"}}
	contract := jsonModelCostContract()
	err := contract.ConfirmCostIdentity(info, []byte(`{"model":"final-override-model"}`))
	require.NoError(t, err)
	assert.Equal(t, "final-override-model", info.BillableUpstreamModel)
}

func TestConfirmCostIdentityFallsBackToMappedModel(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "mapped-model"}}

	err := jsonModelCostContract().ConfirmCostIdentity(info, []byte(`{"messages":[]}`))
	require.NoError(t, err)
	assert.Equal(t, "mapped-model", info.BillableUpstreamModel)
}

func TestCostCapabilitiesExcludeUnsupportedRealtimePath(t *testing.T) {
	covered := CostCapabilitiesForRoute(1, "/v1/chat/completions", "")
	assert.True(t, covered.CanResolveBillableModel)
	assert.Contains(t, covered.ChargeEvents, types.CostChargeResponseSucceeded)
	assert.Contains(t, covered.MeterSources, types.CostMeterUpstreamUsage)

	uncovered := CostCapabilitiesForRoute(1, "/v1/realtime", "")
	assert.False(t, uncovered.CanResolveBillableModel)
	assert.Empty(t, uncovered.ChargeEvents)
	assert.Empty(t, uncovered.MeterSources)
}

func TestNormalizeCostMeterRequiresAuthoritativeBillingUsage(t *testing.T) {
	contract := jsonModelCostContract()
	info := &relaycommon.RelayInfo{}

	_, err := contract.NormalizeCostMeter(info, &dto.Usage{
		BillingUsage: &dto.BillingUsage{
			Estimated:   true,
			OpenAIUsage: &dto.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
		},
	})
	require.Error(t, err)

	meter, err := contract.NormalizeCostMeter(info, &dto.Usage{
		BillingUsage: dto.NewOpenAIChatBillingUsage(&dto.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12}),
	})
	require.NoError(t, err)
	require.NotNil(t, meter.InputTokens)
	require.NotNil(t, meter.OutputTokens)
	require.NotNil(t, meter.TotalTokens)
	assert.Equal(t, int64(10), *meter.InputTokens)
	assert.Equal(t, int64(2), *meter.OutputTokens)
	assert.Equal(t, int64(12), *meter.TotalTokens)
}

func TestStrictCostAdaptorRejectsEmptyIdentityBeforeTransport(t *testing.T) {
	withCostAccountingMode(t, types.CostAccountingStrict)
	fake := &costTransportAdaptor{}
	wrapped := newCostAccountingAdaptor(fake, 1)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, err := wrapped.DoRequest(ctx, &relaycommon.RelayInfo{}, bytes.NewReader([]byte(`{}`)))
	require.Error(t, err)
	assert.False(t, fake.called)
}

func TestDisabledCostAdaptorPreservesExistingTransport(t *testing.T) {
	withCostAccountingMode(t, types.CostAccountingDisabled)
	fake := &costTransportAdaptor{}
	wrapped := newCostAccountingAdaptor(fake, 1)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, err := wrapped.DoRequest(ctx, &relaycommon.RelayInfo{}, bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	assert.True(t, fake.called)
}

func withCostAccountingMode(t *testing.T, mode types.CostAccountingMode) {
	t.Helper()
	cfg := config.GlobalConfig.Get(cost_setting.ConfigName)
	require.NotNil(t, cfg)
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{cost_setting.KeyMode: string(mode)}))
	cost_setting.UpdateAndSync()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{cost_setting.KeyMode: string(types.CostAccountingDisabled)}))
		cost_setting.UpdateAndSync()
	})
}

type costTransportAdaptor struct {
	channel.Adaptor
	called bool
}

func (a *costTransportAdaptor) DoRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ io.Reader) (any, error) {
	a.called = true
	return &http.Response{StatusCode: http.StatusOK}, nil
}

var _ channel.Adaptor = (*costTransportAdaptor)(nil)
