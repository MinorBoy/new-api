package constant_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestDimensioChannelConstants(t *testing.T) {
	require.Equal(t, 200, constant.ChannelTypeDimensio)
	require.Equal(t, 201, constant.ChannelTypeNewAPIVideo)
	require.Equal(t, 202, constant.ChannelTypeClmmMall)
	require.Equal(t, 203, constant.ChannelTypeLucen)
	require.Equal(t, 213, constant.ChannelTypeDummy)
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeSecure)
	require.Equal(t, "https://jimeng.dimensio.cn", constant.ChannelBaseURLs[constant.ChannelTypeDimensio])
	require.Equal(t, "Dimensio", constant.GetChannelTypeName(constant.ChannelTypeDimensio))
	_, success := common.ChannelType2APIType(constant.ChannelTypeDimensio)
	require.False(t, success)
}

func TestNewAPIVideoChannelConstants(t *testing.T) {
	require.Equal(t, 201, constant.ChannelTypeNewAPIVideo)
	require.Equal(t, "", constant.ChannelBaseURLs[constant.ChannelTypeNewAPIVideo])
	require.Equal(t, "NewAPIVideo", constant.GetChannelTypeName(constant.ChannelTypeNewAPIVideo))
	_, success := common.ChannelType2APIType(constant.ChannelTypeNewAPIVideo)
	require.False(t, success)
}

func TestClmmMallChannelConstants(t *testing.T) {
	require.Equal(t, 202, constant.ChannelTypeClmmMall)
	require.Equal(t, 203, constant.ChannelTypeLucen)
	require.Equal(t, 213, constant.ChannelTypeDummy)
	require.Equal(t, "https://clmm-mall.top", constant.ChannelBaseURLs[constant.ChannelTypeClmmMall])
	require.Equal(t, "CLMM Mall", constant.GetChannelTypeName(constant.ChannelTypeClmmMall))
	_, success := common.ChannelType2APIType(constant.ChannelTypeClmmMall)
	require.False(t, success)
}

func TestLucenChannelConstants(t *testing.T) {
	require.Equal(t, 203, constant.ChannelTypeLucen)
	require.Equal(t, "https://lucen.asia", constant.ChannelBaseURLs[constant.ChannelTypeLucen])
	require.Equal(t, "Lucen", constant.GetChannelTypeName(constant.ChannelTypeLucen))
	_, success := common.ChannelType2APIType(constant.ChannelTypeLucen)
	require.False(t, success)
}

func TestMegaByAIChannelConstants(t *testing.T) {
	require.Equal(t, 204, constant.ChannelTypeMegaByAI)
	require.Equal(t, 213, constant.ChannelTypeDummy)
	require.Equal(t, "https://newapi.megabyai.cc", constant.ChannelBaseURLs[constant.ChannelTypeMegaByAI])
	require.Equal(t, "MegaByAI", constant.GetChannelTypeName(constant.ChannelTypeMegaByAI))
	_, success := common.ChannelType2APIType(constant.ChannelTypeMegaByAI)
	require.False(t, success)
}

func TestCangyuanChannelConstants(t *testing.T) {
	require.Equal(t, 205, constant.ChannelTypeCangyuan)
	require.Equal(t, 213, constant.ChannelTypeDummy)
	require.Equal(t, "https://ai.cangyuansuanli.cn", constant.ChannelBaseURLs[constant.ChannelTypeCangyuan])
	require.Equal(t, "Cangyuan", constant.GetChannelTypeName(constant.ChannelTypeCangyuan))
	_, success := common.ChannelType2APIType(constant.ChannelTypeCangyuan)
	require.False(t, success)
}

func TestPaipuChannelConstants(t *testing.T) {
	require.Equal(t, 206, constant.ChannelTypePaipu)
	require.Equal(t, 213, constant.ChannelTypeDummy)
	require.Equal(t, "https://api.paipu.net", constant.ChannelBaseURLs[constant.ChannelTypePaipu])
	require.Equal(t, "Paipu", constant.GetChannelTypeName(constant.ChannelTypePaipu))
	_, success := common.ChannelType2APIType(constant.ChannelTypePaipu)
	require.False(t, success)
}

func TestSecureChannelConstants(t *testing.T) {
	require.Equal(t, 207, constant.ChannelTypeSecure)
	require.Equal(t, 213, constant.ChannelTypeDummy)
	require.Equal(t, "https://token.secure-skill.com", constant.ChannelBaseURLs[constant.ChannelTypeSecure])
	require.Equal(t, "Secure", constant.GetChannelTypeName(constant.ChannelTypeSecure))
}

func TestOmegaAIAndFourSTokenChannelConstants(t *testing.T) {
	require.Equal(t, 208, constant.ChannelTypeOmegaAI)
	require.Equal(t, 209, constant.ChannelTypeFourSToken)
	require.Equal(t, 213, constant.ChannelTypeDummy)
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeFourSToken)
	require.Equal(t, "https://omegaai.xin", constant.ChannelBaseURLs[constant.ChannelTypeOmegaAI])
	require.Equal(t, "https://api.4stoken.cn", constant.ChannelBaseURLs[constant.ChannelTypeFourSToken])
	require.Equal(t, "OmegaAI", constant.GetChannelTypeName(constant.ChannelTypeOmegaAI))
	require.Equal(t, "4stoken", constant.GetChannelTypeName(constant.ChannelTypeFourSToken))
	_, omegaMapped := common.ChannelType2APIType(constant.ChannelTypeOmegaAI)
	require.False(t, omegaMapped)
	_, fourSTokenMapped := common.ChannelType2APIType(constant.ChannelTypeFourSToken)
	require.False(t, fourSTokenMapped)
}

func TestEightYesChannelConstants(t *testing.T) {
	require.Equal(t, 210, constant.ChannelTypeEightYes)
	require.Equal(t, 213, constant.ChannelTypeDummy)
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeEightYes)
	require.Equal(t, "https://8yes.cc", constant.ChannelBaseURLs[constant.ChannelTypeEightYes])
	require.Equal(t, "8yes", constant.GetChannelTypeName(constant.ChannelTypeEightYes))
	_, mapped := common.ChannelType2APIType(constant.ChannelTypeEightYes)
	require.False(t, mapped)
}

func TestZ5APIChannelConstants(t *testing.T) {
	require.Equal(t, 211, constant.ChannelTypeZ5API)
	require.Equal(t, 213, constant.ChannelTypeDummy)
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeZ5API)
	require.Equal(t, "https://z5api.com", constant.ChannelBaseURLs[constant.ChannelTypeZ5API])
	require.Equal(t, "Z5API", constant.GetChannelTypeName(constant.ChannelTypeZ5API))
	_, mapped := common.ChannelType2APIType(constant.ChannelTypeZ5API)
	require.False(t, mapped)
}

func TestFFLinkChannelConstants(t *testing.T) {
	require.Equal(t, 212, constant.ChannelTypeFFLink)
	require.Equal(t, 213, constant.ChannelTypeDummy)
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeFFLink)
	require.Equal(t, "https://api.fflink.top", constant.ChannelBaseURLs[constant.ChannelTypeFFLink])
	require.Equal(t, "FYLink", constant.GetChannelTypeName(constant.ChannelTypeFFLink))
	_, mapped := common.ChannelType2APIType(constant.ChannelTypeFFLink)
	require.False(t, mapped)
}
