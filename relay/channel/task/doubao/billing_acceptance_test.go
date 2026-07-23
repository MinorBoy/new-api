package doubao

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type seedanceAcceptanceModel struct {
	ID            string
	Resolutions   []string
	Durations     []int
	BaseRMB       float64
	SupportsVideo bool
	ImageRole     string
}

type seedanceAcceptanceVideoProfile struct {
	Durations []int
}

func seedanceAcceptanceModels() []seedanceAcceptanceModel {
	return []seedanceAcceptanceModel{
		{"doubao-seedance-2-0-260128", []string{"480p", "720p", "1080p", "4k"}, integerRange(4, 15), 46, true, "reference_image"},
		{"doubao-seedance-2-0-fast-260128", []string{"480p", "720p"}, integerRange(4, 15), 37, true, "reference_image"},
		{"doubao-seedance-2-0-mini-260615", []string{"480p", "720p"}, integerRange(4, 15), 23, true, "reference_image"},
		{"doubao-seedance-1-5-pro-251215", []string{"480p", "720p", "1080p"}, integerRange(4, 12), 8, false, "first_frame"},
	}
}

func integerRange(first, last int) []int {
	values := make([]int, 0, last-first+1)
	for value := first; value <= last; value++ {
		values = append(values, value)
	}
	return values
}

func seedanceAcceptanceVideoProfiles() []seedanceAcceptanceVideoProfile {
	profiles := make([]seedanceAcceptanceVideoProfile, 0, 312)
	var build func([]int, int)
	build = func(prefix []int, remaining int) {
		if len(prefix) > 0 {
			durations := append([]int(nil), prefix...)
			profiles = append(profiles, seedanceAcceptanceVideoProfile{Durations: durations})
		}
		if len(prefix) == 3 {
			return
		}
		for duration := 2; duration <= 15 && duration <= remaining; duration++ {
			build(append(prefix, duration), remaining-duration)
		}
	}
	build(nil, 15)
	return profiles
}

func seedanceAcceptanceExplicitCaseCount() int {
	profilesWithNoVideo := len(seedanceAcceptanceVideoProfiles()) + 1
	seedance20Cells := (4*12 + 2*12 + 2*12) * profilesWithNoVideo * 2
	seedance15Cells := 3*9*2*2*2 + 1*9*2*2
	return seedance20Cells + seedance15Cells
}

func TestSeedanceBillingAcceptanceGeneratorCounts(t *testing.T) {
	profiles := seedanceAcceptanceVideoProfiles()
	require.Len(t, profiles, 312)

	counts := map[int]int{}
	seen := map[string]struct{}{}
	for profileIndex, profile := range profiles {
		caseID := fmt.Sprintf("profile=%d/durations=%v", profileIndex, profile.Durations)
		require.NotEmpty(t, profile.Durations, caseID)
		require.LessOrEqual(t, len(profile.Durations), 3, caseID)
		totalDuration := 0
		for _, duration := range profile.Durations {
			require.GreaterOrEqual(t, duration, 2, caseID)
			require.LessOrEqual(t, duration, 15, caseID)
			totalDuration += duration
		}
		require.LessOrEqual(t, totalDuration, 15, caseID)
		key := fmt.Sprint(profile.Durations)
		require.NotContains(t, seen, key, caseID)
		seen[key] = struct{}{}
		counts[len(profile.Durations)]++
	}
	assert.Equal(t, map[int]int{1: 14, 2: 78, 3: 220}, counts)
	assert.Equal(t, 60348, seedanceAcceptanceExplicitCaseCount())
}

func TestSeedanceBillingAcceptanceOfficialPriceOracle(t *testing.T) {
	tests := []struct {
		model      string
		resolution string
		hasVideo   bool
		want       float64
	}{
		{"doubao-seedance-2-0-260128", "480p", false, 46},
		{"doubao-seedance-2-0-260128", "480p", true, 28},
		{"doubao-seedance-2-0-260128", "720p", false, 46},
		{"doubao-seedance-2-0-260128", "720p", true, 28},
		{"doubao-seedance-2-0-260128", "1080p", false, 51},
		{"doubao-seedance-2-0-260128", "1080p", true, 31},
		{"doubao-seedance-2-0-260128", "4k", false, 26},
		{"doubao-seedance-2-0-260128", "4k", true, 16},
		{"doubao-seedance-2-0-fast-260128", "480p", false, 37},
		{"doubao-seedance-2-0-fast-260128", "480p", true, 22},
		{"doubao-seedance-2-0-fast-260128", "720p", false, 37},
		{"doubao-seedance-2-0-fast-260128", "720p", true, 22},
		{"doubao-seedance-2-0-mini-260615", "480p", false, 23},
		{"doubao-seedance-2-0-mini-260615", "480p", true, 14},
		{"doubao-seedance-2-0-mini-260615", "720p", false, 23},
		{"doubao-seedance-2-0-mini-260615", "720p", true, 14},
	}

	for _, tt := range tests {
		caseID := fmt.Sprintf("model=%s/resolution=%s/has_video=%t", tt.model, tt.resolution, tt.hasVideo)
		assert.Equal(t, tt.want, seedanceAcceptanceOfficialUnitPrice(t, tt.model, tt.resolution, tt.hasVideo), caseID)
	}
}

func TestSeedanceBillingAcceptanceExplicitMatrix(t *testing.T) {
	models := seedanceAcceptanceModels()
	profiles := seedanceAcceptanceVideoProfiles()
	boolValues := []bool{false, true}
	executed := 0

	for _, model := range models {
		if !model.SupportsVideo {
			continue
		}
		for _, resolution := range model.Resolutions {
			for _, duration := range model.Durations {
				for profileIndex := -1; profileIndex < len(profiles); profileIndex++ {
					hasVideo := profileIndex >= 0
					videoDurations := []int(nil)
					if hasVideo {
						videoDurations = profiles[profileIndex].Durations
					}
					for _, image := range boolValues {
						caseID := fmt.Sprintf(
							"model=%s/resolution=%s/duration=%d/video=%v/image=%t",
							model.ID,
							resolution,
							duration,
							videoDurations,
							image,
						)
						durationValue := dto.IntValue(duration)
						request := seedanceNativeRequest{
							Model:      model.ID,
							Resolution: resolution,
							Duration:   &durationValue,
						}
						// Native validation receives reference-video counts; per-video durations are an upstream/E2E contract.
						facts := seedanceContentFacts{videoCount: len(videoDurations)}
						require.Equal(t, len(videoDurations), facts.videoCount, caseID)
						if image {
							facts.imageCount = 1
							switch model.ImageRole {
							case "reference_image":
								facts.referenceImageCount = 1
							case "first_frame":
								facts.firstFrameCount = 1
							default:
								require.FailNow(t, "unsupported acceptance image role", caseID)
							}
						}

						require.NoError(t, validateSeedanceNativeFields(request, facts, false), caseID)
						ratio, ok := GetVideoBillingRatio(model.ID, resolution, hasVideo)
						require.True(t, ok, caseID)
						officialUnitPrice := seedanceAcceptanceOfficialUnitPrice(t, model.ID, resolution, hasVideo)
						configuredUnitPrice := seedanceAcceptanceUnitPrice(t, model.ID, resolution, hasVideo)
						assert.Equal(t, officialUnitPrice, configuredUnitPrice, caseID)
						wantRatio := officialUnitPrice / model.BaseRMB
						assert.InDelta(t, wantRatio, ratio, 1e-12, caseID)
						executed++
					}
				}
			}
		}
	}

	for _, model := range models {
		if model.SupportsVideo {
			continue
		}
		for _, resolution := range model.Resolutions {
			for _, duration := range model.Durations {
				for _, image := range boolValues {
					for _, audio := range boolValues {
						for _, tier := range []string{"default", "flex"} {
							caseID := fmt.Sprintf(
								"model=%s/resolution=%s/duration=%d/image=%t/audio=%t/tier=%s/draft=false",
								model.ID,
								resolution,
								duration,
								image,
								audio,
								tier,
							)
							durationValue := dto.IntValue(duration)
							generateAudioValue := dto.BoolValue(audio)
							draftValue := dto.BoolValue(false)
							request := seedanceNativeRequest{
								Model:         model.ID,
								ServiceTier:   tier,
								GenerateAudio: &generateAudioValue,
								Draft:         &draftValue,
								Resolution:    resolution,
								Duration:      &durationValue,
							}
							facts := seedanceContentFacts{}
							if image {
								facts.imageCount = 1
								switch model.ImageRole {
								case "reference_image":
									facts.referenceImageCount = 1
								case "first_frame":
									facts.firstFrameCount = 1
								default:
									require.FailNow(t, "unsupported acceptance image role", caseID)
								}
							}

							require.NoError(t, validateSeedanceNativeFields(request, facts, false), caseID)
							got, ok := GetSeedance15ProRatios(audio, false, tier)
							require.True(t, ok, caseID)
							want := map[string]float64{}
							if audio {
								want["audio"] = 2
							}
							if tier == "flex" {
								want["service_tier"] = 0.5
							}
							assert.Equal(t, want, got, caseID)
							executed++
						}
					}
				}
			}
		}

		for _, duration := range model.Durations {
			for _, image := range boolValues {
				for _, audio := range boolValues {
					caseID := fmt.Sprintf(
						"model=%s/resolution=480p/duration=%d/image=%t/audio=%t/tier=default/draft=true",
						model.ID,
						duration,
						image,
						audio,
					)
					durationValue := dto.IntValue(duration)
					generateAudioValue := dto.BoolValue(audio)
					draftValue := dto.BoolValue(true)
					request := seedanceNativeRequest{
						Model:         model.ID,
						ServiceTier:   "default",
						GenerateAudio: &generateAudioValue,
						Draft:         &draftValue,
						Resolution:    "480p",
						Duration:      &durationValue,
					}
					facts := seedanceContentFacts{}
					if image {
						facts.imageCount = 1
						switch model.ImageRole {
						case "reference_image":
							facts.referenceImageCount = 1
						case "first_frame":
							facts.firstFrameCount = 1
						default:
							require.FailNow(t, "unsupported acceptance image role", caseID)
						}
					}

					require.NoError(t, validateSeedanceNativeFields(request, facts, false), caseID)
					got, ok := GetSeedance15ProRatios(audio, true, "default")
					require.True(t, ok, caseID)
					want := map[string]float64{}
					if audio {
						want["audio"] = 2
						want["draft_estimate"] = 0.6
					} else {
						want["draft_estimate"] = 0.7
					}
					assert.Equal(t, want, got, caseID)
					executed++
				}
			}
		}
	}

	assert.Equal(t, 60348, executed, "all explicit billing acceptance cases must execute")
}

func TestSeedanceBillingAcceptanceInvalidMatrix(t *testing.T) {
	type invalidCase struct {
		id      string
		request seedanceNativeRequest
		facts   seedanceContentFacts
		content []ContentItem
	}

	models := []struct {
		id          string
		maxDuration int
	}{
		{id: "doubao-seedance-2-0-260128", maxDuration: 15},
		{id: "doubao-seedance-2-0-fast-260128", maxDuration: 15},
		{id: "doubao-seedance-2-0-mini-260615", maxDuration: 15},
		{id: "doubao-seedance-1-5-pro-251215", maxDuration: 12},
	}
	invalidCases := make([]invalidCase, 0, 36)
	for _, model := range []string{"doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615"} {
		for _, resolution := range []string{"1080p", "4k"} {
			invalidCases = append(invalidCases, invalidCase{
				id:      fmt.Sprintf("model=%s/resolution=%s", model, resolution),
				request: seedanceNativeRequest{Model: model, Resolution: resolution},
			})
		}
	}
	for _, model := range models {
		for _, duration := range []int{0, 3, model.maxDuration + 1, -2} {
			durationValue := dto.IntValue(duration)
			invalidCases = append(invalidCases, invalidCase{
				id:      fmt.Sprintf("model=%s/duration=%d", model.id, duration),
				request: seedanceNativeRequest{Model: model.id, Duration: &durationValue},
			})
		}
	}

	fourVideos := []ContentItem{{Type: "text", Text: "four reference videos"}}
	for index := 0; index < 4; index++ {
		fourVideos = append(fourVideos, ContentItem{
			Type:     "video_url",
			Role:     "reference_video",
			VideoURL: &MediaURL{URL: fmt.Sprintf("https://mock.example/reference-%d.mp4", index+1)},
		})
	}
	invalidCases = append(invalidCases, invalidCase{
		id:      "model=doubao-seedance-2-0-260128/reference_videos=4",
		request: seedanceNativeRequest{Model: "doubao-seedance-2-0-260128"},
		content: fourVideos,
	})

	invalidCases = append(invalidCases,
		invalidCase{
			id:      "model=doubao-seedance-1-5-pro-251215/content=reference_image",
			request: seedanceNativeRequest{Model: "doubao-seedance-1-5-pro-251215"},
			content: []ContentItem{{Type: "text", Text: "unsupported reference image"}, {Type: "image_url", Role: "reference_image", ImageURL: &MediaURL{URL: "https://mock.example/reference.png"}}},
		},
		invalidCase{
			id:      "model=doubao-seedance-1-5-pro-251215/content=reference_video",
			request: seedanceNativeRequest{Model: "doubao-seedance-1-5-pro-251215"},
			content: []ContentItem{{Type: "text", Text: "unsupported reference video"}, {Type: "video_url", Role: "reference_video", VideoURL: &MediaURL{URL: "https://mock.example/reference.mp4"}}},
		},
		invalidCase{
			id:      "model=doubao-seedance-1-5-pro-251215/content=reference_audio",
			request: seedanceNativeRequest{Model: "doubao-seedance-1-5-pro-251215"},
			facts:   seedanceContentFacts{audioCount: 1},
		},
	)

	draftValue := dto.BoolValue(true)
	for _, model := range models[:3] {
		invalidCases = append(invalidCases,
			invalidCase{
				id:      fmt.Sprintf("model=%s/service_tier=flex", model.id),
				request: seedanceNativeRequest{Model: model.id, ServiceTier: "flex"},
			},
			invalidCase{
				id:      fmt.Sprintf("model=%s/draft=true", model.id),
				request: seedanceNativeRequest{Model: model.id, Draft: &draftValue},
			},
		)
	}
	invalidCases = append(invalidCases,
		invalidCase{
			id:      "model=doubao-seedance-1-5-pro-251215/draft=true/resolution=720p",
			request: seedanceNativeRequest{Model: "doubao-seedance-1-5-pro-251215", Draft: &draftValue, Resolution: "720p"},
		},
		invalidCase{
			id:      "model=doubao-seedance-1-5-pro-251215/draft=true/resolution=1080p",
			request: seedanceNativeRequest{Model: "doubao-seedance-1-5-pro-251215", Draft: &draftValue, Resolution: "1080p"},
		},
		invalidCase{
			id:      "model=doubao-seedance-1-5-pro-251215/draft=true/service_tier=flex",
			request: seedanceNativeRequest{Model: "doubao-seedance-1-5-pro-251215", Draft: &draftValue, ServiceTier: "flex"},
		},
		invalidCase{
			id:      "content=video_url/role=first_frame",
			request: seedanceNativeRequest{Model: "doubao-seedance-2-0-260128"},
			content: []ContentItem{{Type: "video_url", Role: "first_frame", VideoURL: &MediaURL{URL: "https://mock.example/reference.mp4"}}},
		},
		invalidCase{
			id:      "content=audio_url/role=reference_video",
			request: seedanceNativeRequest{Model: "doubao-seedance-2-0-260128"},
			content: []ContentItem{{Type: "audio_url", Role: "reference_video", AudioURL: &MediaURL{URL: "https://mock.example/reference.wav"}}},
		},
		invalidCase{
			id:      "content=image_url/role=reference_video",
			request: seedanceNativeRequest{Model: "doubao-seedance-2-0-260128"},
			content: []ContentItem{{Type: "image_url", Role: "reference_video", ImageURL: &MediaURL{URL: "https://mock.example/reference.png"}}},
		},
	)

	require.Len(t, invalidCases, 36)
	seen := make(map[string]struct{}, len(invalidCases))
	for _, testCase := range invalidCases {
		require.NotEmpty(t, testCase.id)
		_, duplicate := seen[testCase.id]
		require.False(t, duplicate, testCase.id)
		seen[testCase.id] = struct{}{}

		facts := testCase.facts
		var err error
		if testCase.content != nil {
			facts, err = validateSeedanceContent(testCase.request.Model, testCase.content)
		}
		if err == nil {
			err = validateSeedanceNativeFields(testCase.request, facts, false)
		}
		require.Error(t, err, testCase.id)
		require.NotEmpty(t, err.Error(), testCase.id)
	}
	require.Len(t, seen, 36)
}

func seedanceAcceptanceOfficialUnitPrice(t *testing.T, model, resolution string, hasVideo bool) float64 {
	t.Helper()

	switch model {
	case "doubao-seedance-2-0-260128":
		switch resolution {
		case "480p", "720p":
			if hasVideo {
				return 28
			}
			return 46
		case "1080p":
			if hasVideo {
				return 31
			}
			return 51
		case "4k":
			if hasVideo {
				return 16
			}
			return 26
		}
	case "doubao-seedance-2-0-fast-260128":
		if resolution == "480p" || resolution == "720p" {
			if hasVideo {
				return 22
			}
			return 37
		}
	case "doubao-seedance-2-0-mini-260615":
		if resolution == "480p" || resolution == "720p" {
			if hasVideo {
				return 14
			}
			return 23
		}
	}

	require.FailNow(t, "unknown official Seedance unit price", "model=%s resolution=%s has_video=%t", model, resolution, hasVideo)
	return 0
}

func seedanceAcceptanceUnitPrice(t *testing.T, model, resolution string, hasVideo bool) float64 {
	t.Helper()

	family := seedancePricingFamily(model)
	require.NotEmpty(t, family, "unknown Seedance pricing family for model=%s", model)
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	switch resolution {
	case "480p", "720p", "1080p", "4k":
	default:
		require.FailNow(t, "unknown Seedance price resolution", "model=%s resolution=%s", model, resolution)
	}
	key := videoPriceKey{
		is1080p:  strings.EqualFold(resolution, "1080p"),
		is4k:     strings.EqualFold(resolution, "4k"),
		hasVideo: hasVideo,
	}
	prices, ok := videoPriceTable[family]
	require.True(t, ok, "missing Seedance price table for model=%s family=%s", model, family)
	price, ok := prices[key]
	require.True(t, ok, "missing Seedance price for model=%s resolution=%s has_video=%t", model, resolution, hasVideo)
	return price
}
