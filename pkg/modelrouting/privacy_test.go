package modelrouting_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactsDoNotMarshalReferenceVideoURLs(t *testing.T) {
	resolution := "1080p"
	duration := 10
	ratio := "16:9"
	input := modelrouting.FactsInput{
		CanonicalModel:    modelrouting.Seedance20,
		OutputResolution:  &resolution,
		DurationSeconds:   &duration,
		AspectRatio:       &ratio,
		ReferenceImages:   1,
		ReferenceVideos:   1,
		ReferenceVideoURLs: []string{
			"https://assets.example/a.mp4?sig=secret&token=bearer-value",
			"https://internal.example/b.mov",
		},
	}

	body, err := common.Marshal(input)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "assets.example")
	assert.NotContains(t, string(body), "internal.example")
	assert.NotContains(t, string(body), "sig=secret")
	assert.NotContains(t, string(body), "bearer-value")
	assert.NotContains(t, string(body), "ReferenceVideoURLs")
	assert.NotContains(t, string(body), "reference_video_urls")
}

func TestResolveFactsDoesNotCopyReferenceVideoURLs(t *testing.T) {
	resolution := "1080p"
	duration := 10
	ratio := "16:9"
	input := modelrouting.FactsInput{
		CanonicalModel:    modelrouting.Seedance20,
		OutputResolution:  &resolution,
		DurationSeconds:   &duration,
		AspectRatio:       &ratio,
		ReferenceVideoURLs: []string{"https://assets.example/a.mp4?sig=secret"},
	}

	facts, err := modelrouting.ResolveFacts("group", input, modelrouting.Defaults{
		OutputResolution: "720p",
		DurationSeconds:  5,
		AspectRatio:      "16:9",
	})
	require.NoError(t, err)

	body, err := common.Marshal(facts)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "assets.example")
	assert.NotContains(t, string(body), "sig=secret")
}

func TestAuditDoesNotMarshalReferenceVideoURLs(t *testing.T) {
	resolution := "1080p"
	duration := 10
	ratio := "16:9"
	facts, err := modelrouting.ResolveFacts("group", modelrouting.FactsInput{
		CanonicalModel:    modelrouting.Seedance20,
		OutputResolution:  &resolution,
		DurationSeconds:   &duration,
		AspectRatio:       &ratio,
		ReferenceVideoURLs: []string{"https://assets.example/secret.mp4?sig=token"},
	}, modelrouting.Defaults{
		OutputResolution: "720p",
		DurationSeconds:  5,
		AspectRatio:      "16:9",
	})
	require.NoError(t, err)

	audit := modelrouting.Audit{
		PolicyID:      1,
		TargetID:      2,
		TargetName:    "target",
		UpstreamModel: "upstream",
		Facts:         facts,
		MismatchCounts: map[modelrouting.MismatchReason]int{
			modelrouting.MismatchResolution: 1,
		},
	}

	body, err := common.Marshal(audit)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "assets.example")
	assert.NotContains(t, string(body), "sig=token")
}
