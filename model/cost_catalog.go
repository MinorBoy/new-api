package model

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CostCatalogQuery struct {
	ChannelID             int
	BillableUpstreamModel string
	CostMode              string
	Status                string
	Source                string
	SortBy                string
	SortOrder             string
	Offset                int
	Limit                 int
}

type CostCatalogRow struct {
	ChannelModelCostRule
	ChannelName    string `gorm:"column:channel_name"`
	ChannelType    int    `gorm:"column:channel_type"`
	ChannelMissing bool   `gorm:"column:channel_missing"`
}

var costCatalogSortColumns = map[string]clause.Column{
	"channel_id":              {Table: "rules", Name: "channel_id"},
	"billable_upstream_model": {Table: "rules", Name: "billable_upstream_model"},
	"cost_variant_key":        {Table: "rules", Name: "cost_variant_key"},
	"status":                  {Table: "rules", Name: "status"},
	"version":                 {Table: "rules", Name: "version"},
	"cost_mode":               {Table: "rules", Name: "cost_mode"},
	"source":                  {Table: "rules", Name: "source"},
	"effective_from":          {Table: "rules", Name: "effective_from"},
}

func ListCostCatalogRows(query CostCatalogQuery) ([]CostCatalogRow, int64, error) {
	db := costCatalogBaseQuery(query)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	db = applyCostCatalogOrder(db, query.SortBy, query.SortOrder)
	if query.Offset > 0 {
		db = db.Offset(query.Offset)
	}
	if query.Limit > 0 {
		db = db.Limit(query.Limit)
	}
	rows := make([]CostCatalogRow, 0)
	if err := db.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func WalkCostCatalogRows(query CostCatalogQuery, batchSize int, visit func([]CostCatalogRow) error) error {
	if batchSize <= 0 {
		batchSize = 500
	}
	offset := query.Offset
	for {
		rows := make([]CostCatalogRow, 0, batchSize)
		db := applyCostCatalogOrder(costCatalogBaseQuery(query), query.SortBy, query.SortOrder).
			Offset(offset).
			Limit(batchSize)
		if err := db.Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		if visit != nil {
			if err := visit(rows); err != nil {
				return err
			}
		}
		if len(rows) < batchSize {
			return nil
		}
		offset += len(rows)
	}
}

func GetCostCatalogRow(id int64) (*CostCatalogRow, error) {
	var row CostCatalogRow
	err := costCatalogBaseQuery(CostCatalogQuery{Status: "all"}).
		Where("rules.id = ?", id).
		First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func ListCostCatalogHistoryRows(channelID int, billableModel, variant string) ([]CostCatalogRow, error) {
	rows := make([]CostCatalogRow, 0)
	err := costCatalogBaseQuery(CostCatalogQuery{ChannelID: channelID, Status: "all"}).
		Where("rules.billable_upstream_model = ? AND rules.cost_variant_key = ?", billableModel, variant).
		Order(clause.OrderByColumn{Column: clause.Column{Table: "rules", Name: "version"}, Desc: true}).
		Order(clause.OrderByColumn{Column: clause.Column{Table: "rules", Name: "id"}, Desc: true}).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func costCatalogBaseQuery(query CostCatalogQuery) *gorm.DB {
	ruleTable := DB.NamingStrategy.TableName("ChannelModelCostRule")
	channelTable := DB.NamingStrategy.TableName("Channel")
	db := DB.Table(ruleTable+" AS rules").
		Select("rules.*, COALESCE(channels.name, '') AS channel_name, COALESCE(channels.type, 0) AS channel_type, CASE WHEN channels.id IS NULL THEN ? ELSE ? END AS channel_missing", true, false).
		Joins("LEFT JOIN " + channelTable + " AS channels ON channels.id = rules.channel_id")

	if query.ChannelID > 0 {
		db = db.Where("rules.channel_id = ?", query.ChannelID)
	}
	if query.BillableUpstreamModel != "" {
		literal := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(query.BillableUpstreamModel))
		db = db.Where("LOWER(rules.billable_upstream_model) LIKE ? ESCAPE '!'", "%"+literal+"%")
	}
	if query.CostMode != "" {
		db = db.Where("rules.cost_mode = ?", query.CostMode)
	}
	if query.Status != "" && query.Status != "all" {
		db = db.Where("rules.status = ?", query.Status)
	}
	if query.Source != "" {
		db = db.Where("rules.source = ?", query.Source)
	}
	return db
}

func applyCostCatalogOrder(db *gorm.DB, sortBy, sortOrder string) *gorm.DB {
	descending := strings.EqualFold(sortOrder, "desc")
	if sortBy == "" || sortBy == "channel_name" {
		db = db.Order("CASE WHEN channels.id IS NULL THEN 1 ELSE 0 END ASC")
		db = db.Order(clause.OrderByColumn{
			Column: clause.Column{Table: "channels", Name: "name"},
			Desc:   descending,
		})
		if sortBy == "" {
			db = db.
				Order(clause.OrderByColumn{Column: clause.Column{Table: "rules", Name: "channel_id"}}).
				Order(clause.OrderByColumn{Column: clause.Column{Table: "rules", Name: "billable_upstream_model"}}).
				Order(clause.OrderByColumn{Column: clause.Column{Table: "rules", Name: "cost_variant_key"}}).
				Order(clause.OrderByColumn{Column: clause.Column{Table: "rules", Name: "version"}, Desc: true})
		}
	} else if column, ok := costCatalogSortColumns[sortBy]; ok {
		db = db.Order(clause.OrderByColumn{Column: column, Desc: descending})
	} else {
		db = db.Order(clause.OrderByColumn{Column: clause.Column{Table: "rules", Name: "id"}})
		return db
	}
	return db.Order(clause.OrderByColumn{Column: clause.Column{Table: "rules", Name: "id"}})
}
