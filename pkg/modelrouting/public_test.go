package modelrouting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPublicSeedanceModelRequiresExactCanonicalID(t *testing.T) {
	require.Contains(t, CanonicalModels, Seedance25)
	assert.True(t, IsPublicSeedanceModel(Seedance25))
	for _, modelName := range CanonicalModels {
		assert.True(t, IsPublicSeedanceModel(modelName), modelName)
	}
	for _, modelName := range []string{
		"doubao-seedance-2-0-mini-260128",
		"mg-seedance2.0-480p-fast-gz-15s",
		"GPT-5",
		"claude-sonnet-4-5",
		"",
	} {
		assert.False(t, IsPublicSeedanceModel(modelName), modelName)
	}
}

func TestIsHiddenSeedanceModelOnlyRejectsNonPublicSeedanceIDs(t *testing.T) {
	SetHiddenSeedanceModels([]string{"4sdance431", "videos-fast", "video-2.0-pro"})
	t.Cleanup(func() { SetHiddenSeedanceModels(nil) })

	for _, modelName := range []string{
		"doubao-seedance-2-0-mini-260128",
		"mg-seedance2.0-480p-fast-gz-15s",
		"BB-SEEDANCE-2.0-PRO",
		"4sdance431",
		"videos-fast",
		"video-2.0-pro",
	} {
		assert.True(t, IsHiddenSeedanceModel(modelName), modelName)
	}

	for _, modelName := range append([]string{
		"gpt-5",
		"gpt-image-2",
		"claude-sonnet-4-5",
		"gemini-2.5-pro",
		"deepseek-chat",
		"glm-4.5",
		"",
	}, CanonicalModels...) {
		assert.False(t, IsHiddenSeedanceModel(modelName), modelName)
	}
}

func TestFilterPublicModelsKeepsNonSeedanceAndPublicSeedanceInInputOrder(t *testing.T) {
	SetHiddenSeedanceModels([]string{"4sdance431", "videos-fast", "video-2.0-pro"})
	t.Cleanup(func() { SetHiddenSeedanceModels(nil) })

	actual := FilterPublicModels([]string{
		"gpt-5",
		"mg-seedance2.0-480p-fast-gz-15s",
		"4sdance431",
		"videos-fast",
		"video-2.0-pro",
		Seedance20Mini,
		"claude-sonnet-4-5",
		Seedance20,
		"gpt-5",
		Seedance20Mini,
		Seedance20Fast,
		"doubao-seedance-2-0-mini-260128",
	})

	require.Equal(t, []string{
		"gpt-5",
		Seedance20Mini,
		"claude-sonnet-4-5",
		Seedance20,
		Seedance20Fast,
	}, actual)
}

func TestOrderPublicModelsUsesCanonicalSeedanceOrderWithoutMovingOtherModels(t *testing.T) {
	actual := OrderPublicModels([]string{
		"gpt-5",
		Seedance25,
		Seedance20Mini,
		"claude-sonnet-4-5",
		Seedance20,
		Seedance20Fast,
	})

	require.Equal(t, []string{
		"gpt-5",
		Seedance20,
		Seedance20Fast,
		"claude-sonnet-4-5",
		Seedance20Mini,
		Seedance25,
	}, actual)
}
