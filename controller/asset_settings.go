package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type secureAssetChannelOption struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Status  int    `json:"status"`
	Default bool   `json:"default"`
}

func GetSecureAssetSettings(c *gin.Context) {
	defaultChannelID := secureAssetDefaultChannelID()
	var channels []model.Channel
	if err := model.DB.Where("type = ? AND status = ?", constant.ChannelTypeSecure, common.ChannelStatusEnabled).
		Order("id ASC").Find(&channels).Error; err != nil {
		writeSecureAssetSettingsError(c, http.StatusInternalServerError, "failed to load Secure asset channels")
		return
	}
	options := make([]secureAssetChannelOption, 0, len(channels))
	for i := range channels {
		if !isEligibleSecureAssetChannel(&channels[i]) {
			continue
		}
		options = append(options, secureAssetChannelOption{
			ID:      channels[i].Id,
			Name:    channels[i].Name,
			Status:  channels[i].Status,
			Default: channels[i].Id == defaultChannelID,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"default_channel_id": defaultChannelID,
			"channels":           options,
		},
	})
}

func UpdateSecureAssetSettings(c *gin.Context) {
	var request struct {
		ChannelID int `json:"channel_id"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.ChannelID <= 0 {
		writeSecureAssetSettingsError(c, http.StatusBadRequest, "a valid Secure enterprise channel is required")
		return
	}
	var channel model.Channel
	if err := model.DB.First(&channel, "id = ?", request.ChannelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeSecureAssetSettingsError(c, http.StatusBadRequest, "Secure asset channel not found")
			return
		}
		writeSecureAssetSettingsError(c, http.StatusInternalServerError, "failed to load Secure asset channel")
		return
	}
	if !isEligibleSecureAssetChannel(&channel) {
		writeSecureAssetSettingsError(c, http.StatusBadRequest, "channel must be an enabled single-key Secure enterprise channel")
		return
	}
	if err := model.UpdateOption(service.SecureAssetDefaultChannelOptionKey, strconv.Itoa(channel.Id)); err != nil {
		writeSecureAssetSettingsError(c, http.StatusInternalServerError, "failed to save Secure asset settings")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"default_channel_id": channel.Id},
	})
}

func isEligibleSecureAssetChannel(channel *model.Channel) bool {
	if channel == nil || channel.Type != constant.ChannelTypeSecure || channel.Status != common.ChannelStatusEnabled || channel.ChannelInfo.IsMultiKey {
		return false
	}
	settings := channel.GetOtherSettings()
	if settings.SecureVideoGroup != relaydto.SecureVideoGroupEnterprise {
		return false
	}
	keys := channel.GetKeys()
	return len(keys) == 1 && strings.TrimSpace(keys[0]) != ""
}

func secureAssetDefaultChannelID() int {
	var option model.Option
	if err := model.DB.Where(&model.Option{Key: service.SecureAssetDefaultChannelOptionKey}).First(&option).Error; err != nil {
		return 0
	}
	channelID, err := strconv.Atoi(strings.TrimSpace(option.Value))
	if err != nil {
		return 0
	}
	return channelID
}

func writeSecureAssetSettingsError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"success": false, "code": "secure_asset_settings_error", "message": message})
}
