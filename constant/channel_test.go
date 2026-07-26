package constant_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestDimensioChannelConstants(t *testing.T) {
	require.Equal(t, 59, constant.ChannelTypeDimensio)
	require.Equal(t, 60, constant.ChannelTypeNewAPIVideo)
	require.Equal(t, 61, constant.ChannelTypeClmmMall)
	require.Equal(t, 62, constant.ChannelTypeLucen)
	require.Equal(t, 66, constant.ChannelTypeDummy)
	require.Equal(t, "https://jimeng.dimensio.cn", constant.ChannelBaseURLs[constant.ChannelTypeDimensio])
	require.Equal(t, "Dimensio", constant.GetChannelTypeName(constant.ChannelTypeDimensio))
	_, success := common.ChannelType2APIType(constant.ChannelTypeDimensio)
	require.False(t, success)
}

func TestNewAPIVideoChannelConstants(t *testing.T) {
	require.Equal(t, 60, constant.ChannelTypeNewAPIVideo)
	require.Equal(t, "", constant.ChannelBaseURLs[constant.ChannelTypeNewAPIVideo])
	require.Equal(t, "NewAPIVideo", constant.GetChannelTypeName(constant.ChannelTypeNewAPIVideo))
	_, success := common.ChannelType2APIType(constant.ChannelTypeNewAPIVideo)
	require.False(t, success)
}

func TestClmmMallChannelConstants(t *testing.T) {
	require.Equal(t, 61, constant.ChannelTypeClmmMall)
	require.Equal(t, 62, constant.ChannelTypeLucen)
	require.Equal(t, 66, constant.ChannelTypeDummy)
	require.Equal(t, "https://clmm-mall.top", constant.ChannelBaseURLs[constant.ChannelTypeClmmMall])
	require.Equal(t, "CLMM Mall", constant.GetChannelTypeName(constant.ChannelTypeClmmMall))
	_, success := common.ChannelType2APIType(constant.ChannelTypeClmmMall)
	require.False(t, success)
}

func TestLucenChannelConstants(t *testing.T) {
	require.Equal(t, 62, constant.ChannelTypeLucen)
	require.Equal(t, "https://lucen.asia", constant.ChannelBaseURLs[constant.ChannelTypeLucen])
	require.Equal(t, "Lucen", constant.GetChannelTypeName(constant.ChannelTypeLucen))
	_, success := common.ChannelType2APIType(constant.ChannelTypeLucen)
	require.False(t, success)
}

func TestMegaByAIChannelConstants(t *testing.T) {
	require.Equal(t, 63, constant.ChannelTypeMegaByAI)
	require.Equal(t, 66, constant.ChannelTypeDummy)
	require.Equal(t, "https://newapi.megabyai.cc", constant.ChannelBaseURLs[constant.ChannelTypeMegaByAI])
	require.Equal(t, "MegaByAI", constant.GetChannelTypeName(constant.ChannelTypeMegaByAI))
	_, success := common.ChannelType2APIType(constant.ChannelTypeMegaByAI)
	require.False(t, success)
}

func TestCangyuanChannelConstants(t *testing.T) {
	require.Equal(t, 64, constant.ChannelTypeCangyuan)
	require.Equal(t, 66, constant.ChannelTypeDummy)
	require.Equal(t, "https://ai.cangyuansuanli.cn", constant.ChannelBaseURLs[constant.ChannelTypeCangyuan])
	require.Equal(t, "Cangyuan", constant.GetChannelTypeName(constant.ChannelTypeCangyuan))
	_, success := common.ChannelType2APIType(constant.ChannelTypeCangyuan)
	require.False(t, success)
}

func TestPaipuChannelConstants(t *testing.T) {
	require.Equal(t, 65, constant.ChannelTypePaipu)
	require.Equal(t, 66, constant.ChannelTypeDummy)
	require.Equal(t, "https://api.paipu.net", constant.ChannelBaseURLs[constant.ChannelTypePaipu])
	require.Equal(t, "Paipu", constant.GetChannelTypeName(constant.ChannelTypePaipu))
	_, success := common.ChannelType2APIType(constant.ChannelTypePaipu)
	require.False(t, success)
}
