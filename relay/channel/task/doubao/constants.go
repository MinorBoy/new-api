package doubao

import "strings"

const (
	seedance20Family     = "seedance-2.0"
	seedance20FastFamily = "seedance-2.0-fast"
	seedance20MiniFamily = "seedance-2.0-mini"
	seedance15ProFamily  = "seedance-1.5-pro"
)

var ModelList = []string{
	"doubao-seedance-1-0-pro-250528",
	"doubao-seedance-1-0-lite-t2v",
	"doubao-seedance-1-0-lite-i2v",
	"doubao-seedance-1-5-pro-251215",
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-fast-260128",
	"doubao-seedance-2-0-mini-260615",
}

var ChannelName = "doubao-video"

// videoPriceKey 价格表的键：输出分辨率档（is1080p/is4k 均为 false 即 480p/720p 基准档）、输入是否含视频。
type videoPriceKey struct {
	is1080p  bool
	is4k     bool
	hasVideo bool
}

// videoPriceTable 各模型在不同 (输出分辨率档, 是否含视频输入) 下的单价（元/百万 token）。
// 其中零值键 {480p/720p, 不含视频} 为基准价，等于管理员应配置的 ModelRatio；
// 计费时取 实际单价/基准价 作为 OtherRatio。
var videoPriceTable = map[string]map[videoPriceKey]float64{
	seedance20Family: {
		{hasVideo: false}:                46.0,
		{hasVideo: true}:                 28.0,
		{is1080p: true, hasVideo: false}: 51.0,
		{is1080p: true, hasVideo: true}:  31.0,
		{is4k: true, hasVideo: false}:    26.0,
		{is4k: true, hasVideo: true}:     16.0,
	},
	seedance20FastFamily: {
		{hasVideo: false}: 37.0,
		{hasVideo: true}:  22.0,
	},
	seedance20MiniFamily: {
		{hasVideo: false}: 23.0,
		{hasVideo: true}:  14.0,
	},
}

var videoBasePrices = map[string]float64{
	seedance20Family:     46,
	seedance20FastFamily: 37,
	seedance20MiniFamily: 23,
}

func seedancePricingFamily(modelName string) string {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.HasPrefix(modelName, "doubao-seedance-2-0-fast"):
		return seedance20FastFamily
	case strings.HasPrefix(modelName, "doubao-seedance-2-0-mini"):
		return seedance20MiniFamily
	case strings.HasPrefix(modelName, "doubao-seedance-2-0"):
		return seedance20Family
	case strings.HasPrefix(modelName, "doubao-seedance-1-5-pro"):
		return seedance15ProFamily
	default:
		return ""
	}
}

// GetVideoInputRatio 返回指定模型在给定输出分辨率/是否含视频输入下，相对基准价的计费倍率。
// 第二个返回值表示该模型是否配置了价格表；倍率为 1.0 时调用方可忽略该 OtherRatio。
func GetVideoInputRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	family := seedancePricingFamily(modelName)
	prices, ok := videoPriceTable[family]
	base := videoBasePrices[family]
	if !ok || base <= 0 {
		return 0, false
	}
	res := strings.ToLower(strings.TrimSpace(resolution))
	if res == "" {
		res = "720p"
	}
	if res != "480p" && res != "720p" && res != "1080p" && res != "4k" {
		return 0, false
	}
	price, ok := prices[videoPriceKey{is1080p: res == "1080p", is4k: res == "4k", hasVideo: hasVideo}]
	if !ok {
		return 0, false
	}
	return price / base, true
}

// GetVideoBillingRatio is the descriptive alias used by billing callers.
func GetVideoBillingRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	return GetVideoInputRatio(modelName, resolution, hasVideo)
}

func GetSeedance15ProRatios(generateAudio, draft bool, serviceTier string) (map[string]float64, bool) {
	serviceTier = strings.ToLower(strings.TrimSpace(serviceTier))
	if serviceTier == "" {
		serviceTier = "default"
	}
	if serviceTier != "default" && serviceTier != "flex" {
		return nil, false
	}
	ratios := make(map[string]float64)
	if generateAudio {
		ratios["audio"] = 2
	}
	if serviceTier == "flex" {
		ratios["service_tier"] = 0.5
	}
	if draft {
		if generateAudio {
			ratios["draft_estimate"] = 0.6
		} else {
			ratios["draft_estimate"] = 0.7
		}
	}
	return ratios, true
}
