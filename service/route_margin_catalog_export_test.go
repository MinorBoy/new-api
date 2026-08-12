package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteRouteMarginCatalogCSVIncludesMarginFields(t *testing.T) {
	prepareRouteMarginCatalogServiceDB(t)
	policy := seedRouteMarginPolicyTarget(t, "route-target/export", "export-model", "720p")
	require.NoError(t, model.DB.Create(&model.RouteTarget{
		PolicyID: policy.ID, ChannelID: 7, Name: "route-target/missing", UpstreamModel: "missing-model",
		CostVariantKey: "default", Constraints: `{"output_resolutions":["720p"]}`,
		Enabled: true, ManagedBy: string(types.RouteTargetManagedByConfigImport),
	}).Error)
	seedRouteMarginRule(t, 7, "export-model", types.CostModePerRequest, types.CostRuleConfigV1{
		UnitPrice: stringPointer("0.7"), ChargeEvent: types.CostChargeResponseSucceeded,
	})
	installRouteMarginRevenueHook(t, 500_000, "500000")

	var output bytes.Buffer
	count, err := WriteRouteMarginCatalogCSV(&output, RouteMarginCatalogFilter{
		MinimumMarginPPM: 300000, DurationSeconds: 4, GroupRatio: 1,
		Scenario: RouteMarginScenarioNoVideo, Page: 1, PageSize: 50,
	}, context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.True(t, bytes.HasPrefix(output.Bytes(), []byte{0xEF, 0xBB, 0xBF}))
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(output.Bytes(), []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3)
	assert.Equal(t, []string{
		"路由目标", "渠道名称", "渠道 ID", "规范模型", "上游模型", "成本变体",
		"分辨率", "场景", "输出时长（秒）", "分组倍率", "成本模式",
		"预计收入（USD）", "预计成本（USD）", "预计利润（USD）", "毛利率",
		"达标状态", "不通过原因", "成本规则来源", "用户价格来源", "规则 ID", "规则版本",
	}, records[0])
	require.Len(t, records[1], len(routeMarginCatalogCSVHeader))
	assert.Equal(t, "route-target/export", records[1][0])
	assert.Equal(t, "1", records[1][11])
	assert.Equal(t, "0.7", records[1][12])
	assert.Equal(t, "0.3", records[1][13])
	assert.Equal(t, "30%", records[1][14])
	assert.Equal(t, "true", records[1][15])
	assert.Empty(t, records[1][16])
	require.Len(t, records[2], len(routeMarginCatalogCSVHeader))
	assert.Equal(t, "route-target/missing", records[2][0])
	assert.Equal(t, string(ProfitReasonCostRuleMissing), records[2][16])
}

func TestWriteRouteMarginCatalogCSVCalculatesEachRowOnce(t *testing.T) {
	prepareRouteMarginCatalogServiceDB(t)
	policy := seedRouteMarginPolicyTarget(t, "route-target/000", "export-model", "720p")
	for index := 1; index < 51; index++ {
		require.NoError(t, model.DB.Create(&model.RouteTarget{
			PolicyID: policy.ID, ChannelID: 7, Name: fmt.Sprintf("route-target/%03d", index),
			UpstreamModel: "export-model", CostVariantKey: "default",
			Constraints: `{"output_resolutions":["720p"]}`, Enabled: true,
			ManagedBy: string(types.RouteTargetManagedByConfigImport),
		}).Error)
	}
	seedRouteMarginRule(t, 7, "export-model", types.CostModePerRequest, types.CostRuleConfigV1{
		UnitPrice: stringPointer("0.7"), ChargeEvent: types.CostChargeResponseSucceeded,
	})
	previewCalls := 0
	previousHook := RevenuePreviewHookForTest()
	SetRoutingRevenuePreview(func(context.Context, RoutingRevenuePreviewInput) (int64, string, error) {
		previewCalls++
		return 500_000, "500000", nil
	})
	t.Cleanup(func() { SetRoutingRevenuePreview(previousHook) })

	var output bytes.Buffer
	count, err := WriteRouteMarginCatalogCSV(&output, RouteMarginCatalogFilter{
		MinimumMarginPPM: 300000, DurationSeconds: 4, GroupRatio: 1,
		Scenario: RouteMarginScenarioAll, Page: 1, PageSize: 50,
	}, context.Background())

	require.NoError(t, err)
	assert.Equal(t, 102, count)
	assert.Equal(t, count, previewCalls)
}

type routeMarginCatalogFailingWriter struct{}

func (routeMarginCatalogFailingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestWriteRouteMarginCatalogCSVWrapsWriterErrors(t *testing.T) {
	prepareRouteMarginCatalogServiceDB(t)
	_, err := WriteRouteMarginCatalogCSV(routeMarginCatalogFailingWriter{}, RouteMarginCatalogFilter{
		MinimumMarginPPM: 300000, DurationSeconds: 4, GroupRatio: 1,
		Scenario: RouteMarginScenarioAll, Page: 1, PageSize: 50,
	}, context.Background())

	require.ErrorIs(t, err, ErrRouteMarginCatalogUnavailable)
}
