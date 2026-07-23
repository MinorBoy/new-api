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
	require.Equal(t, 62, constant.ChannelTypeDummy)
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
	require.Equal(t, 62, constant.ChannelTypeDummy)
	require.Equal(t, "https://clmm-mall.top", constant.ChannelBaseURLs[constant.ChannelTypeClmmMall])
	require.Equal(t, "CLMM Mall", constant.GetChannelTypeName(constant.ChannelTypeClmmMall))
	_, success := common.ChannelType2APIType(constant.ChannelTypeClmmMall)
	require.False(t, success)
}
