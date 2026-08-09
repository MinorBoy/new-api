package service

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteSupplierCostCatalogCSVUsesFixedColumnsAndBOM(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)

	var output bytes.Buffer
	count, err := WriteSupplierCostCatalogCSV(&output, CostCatalogFilter{}, CostCatalogExportAll)
	require.NoError(t, err)
	assert.Equal(t, 11, count)
	assert.True(t, bytes.HasPrefix(output.Bytes(), []byte{0xEF, 0xBB, 0xBF}))
	records := readCostCatalogCSV(t, output.Bytes())
	require.Len(t, records, 12)
	assert.Equal(t, "渠道名称", records[0][0])
	assert.Equal(t, "价格状态", records[0][len(records[0])-2])
	assert.Equal(t, "问题代码", records[0][len(records[0])-1])
	for _, record := range records[1:] {
		assert.Len(t, record, len(records[0]))
	}
}

func TestWriteSupplierCostCatalogCSVNeutralizesSpreadsheetFormulas(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 5, Name: "=cmd|' /C calc'!A0", Type: 1, Key: "secret"}).Error)
	config := validCatalogCostConfig("USD")
	config.UnitPrice = catalogStringPointer("1")
	rule := catalogCostRule(t, 5, " +SUM(1,1)", 1, types.CostRuleActive, types.CostModePerRequest, config, "manual")
	rule.Note = "\t@SUM(1,1)"
	require.NoError(t, model.DB.Create(&rule).Error)

	var output bytes.Buffer
	_, err := WriteSupplierCostCatalogCSV(&output, CostCatalogFilter{}, CostCatalogExportAll)
	require.NoError(t, err)
	records := readCostCatalogCSV(t, output.Bytes())
	require.Len(t, records, 2)
	columns := costCatalogCSVColumnIndexes(records[0])
	assert.True(t, strings.HasPrefix(records[1][columns["渠道名称"]], "'"))
	assert.True(t, strings.HasPrefix(records[1][columns["计费上游模型"]], "'"))
	assert.True(t, strings.HasPrefix(records[1][columns["备注"]], "'"))
}

func TestWriteSupplierCostCatalogCSVLeavesUnknownPricesBlank(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)

	var output bytes.Buffer
	_, err := WriteSupplierCostCatalogCSV(&output, CostCatalogFilter{Status: "active"}, CostCatalogExportFiltered)
	require.NoError(t, err)
	records := readCostCatalogCSV(t, output.Bytes())
	columns := costCatalogCSVColumnIndexes(records[0])
	for _, record := range records[1:] {
		modelName := record[columns["计费上游模型"]]
		if modelName != "invalid-config-model" && modelName != "missing-normalized-model" {
			continue
		}
		assert.Empty(t, record[columns["每次原币单价"]])
		assert.Empty(t, record[columns["每次标准化 USD 单价"]])
		assert.Equal(t, "unavailable", record[columns["价格状态"]])
	}
}

func TestWriteSupplierCostCatalogCSVLabelsComparisonOnly(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)

	var output bytes.Buffer
	_, err := WriteSupplierCostCatalogCSV(&output, CostCatalogFilter{Status: "active"}, CostCatalogExportFiltered)
	require.NoError(t, err)
	records := readCostCatalogCSV(t, output.Bytes())
	columns := costCatalogCSVColumnIndexes(records[0])
	comparisonColumn, ok := columns["15 秒等效 USD/秒（仅比较）"]
	require.True(t, ok)
	for _, record := range records[1:] {
		switch record[columns["计费上游模型"]] {
		case "per-request-model":
			assert.Equal(t, "0.2", record[comparisonColumn])
		case "per-duration-model":
			assert.Empty(t, record[comparisonColumn])
		}
	}
}

func TestWriteSupplierCostCatalogCSVHonorsFilteredAndAllScopes(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)
	filter := CostCatalogFilter{ChannelID: 1, Status: "active", Source: "config_import", Currency: "USD"}

	var filtered bytes.Buffer
	filteredCount, err := WriteSupplierCostCatalogCSV(&filtered, filter, CostCatalogExportFiltered)
	require.NoError(t, err)
	assert.Equal(t, 1, filteredCount)

	var all bytes.Buffer
	allCount, err := WriteSupplierCostCatalogCSV(&all, filter, CostCatalogExportAll)
	require.NoError(t, err)
	assert.Equal(t, 11, allCount)
}

func TestWriteSupplierCostCatalogCSVExcludesSensitiveChannelFields(t *testing.T) {
	prepareCostCatalogServiceDB(t)
	seedCostCatalogServiceRows(t)
	baseURL := "https://secret-base-url.example"
	headerOverride := "secret-header-override"
	setting := "secret-setting"
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 1).Updates(map[string]any{
		"base_url": baseURL, "header_override": headerOverride, "setting": setting,
	}).Error)
	var rule model.ChannelModelCostRule
	require.NoError(t, model.DB.Where("billable_upstream_model = ? AND status = ?", "per-request-model", types.CostRuleActive).First(&rule).Error)
	rule.ConfigJSON = strings.TrimSuffix(rule.ConfigJSON, "}") + `,"secret_config_marker":"do-not-export"}`
	require.NoError(t, model.DB.Model(&rule).Update("config_json", rule.ConfigJSON).Error)

	var output bytes.Buffer
	_, err := WriteSupplierCostCatalogCSV(&output, CostCatalogFilter{}, CostCatalogExportAll)
	require.NoError(t, err)
	for _, secret := range []string{"alpha-secret", baseURL, headerOverride, setting, "do-not-export"} {
		assert.NotContains(t, output.String(), secret)
	}
}

func readCostCatalogCSV(t *testing.T, data []byte) [][]string {
	t.Helper()
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	require.NoError(t, err)
	return records
}

func costCatalogCSVColumnIndexes(header []string) map[string]int {
	columns := make(map[string]int, len(header))
	for index, name := range header {
		columns[name] = index
	}
	return columns
}
