package newapivideo

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

const ChannelNameLucen = "Lucen"

const ChannelNameMegaByAI = "MegaByAI"

const ChannelNameCangyuan = "Cangyuan"

const ChannelNamePaipu = "Paipu"

const ChannelNameSecure = "Secure"

const ChannelNameOmegaAI = "OmegaAI"

const ChannelNameFourSToken = "4stoken"

const ChannelNameEightYes = "8yes"

const ChannelNameZ5API = "Z5API"

const ChannelNameZZone = "ZZone"
const ChannelNameMikoto = "Mikoto"

type videoRequestDialect string

const (
	videoRequestDialectNewAPIGenerations   videoRequestDialect = "newapi_generations"
	videoRequestDialectMegaReferenceArrays videoRequestDialect = "mega_reference_arrays"
	videoRequestDialectTextJSON            videoRequestDialect = "text_json"
	videoRequestDialectCangyuanMedia       videoRequestDialect = "cangyuan_media"
	videoRequestDialectSecureDiscount      videoRequestDialect = "secure_discount"
	videoRequestDialectSecureOverseas      videoRequestDialect = "secure_overseas"
	videoRequestDialectSecureEnterprise    videoRequestDialect = "secure_enterprise"
	videoRequestDialectOmegaMediaArrays    videoRequestDialect = "omega_media_arrays"
	videoRequestDialectFourSToken          videoRequestDialect = "fourstoken"
	videoRequestDialectEightYes            videoRequestDialect = "eightyes"
	videoRequestDialectPaipuMediaArrays    videoRequestDialect = "paipu_media_arrays"
	videoRequestDialectZ5APIMedia          videoRequestDialect = "z5api_media"
	videoRequestDialectZZone               videoRequestDialect = "zzone"
	videoRequestDialectMikoto              videoRequestDialect = "mikoto"
)

type omegaRequestProfile struct {
	MaxImages int
	MaxVideos int
	MaxAudios int
}

type textRequestProfile struct {
	ratioField                   string
	minimumDuration              int
	maximumDuration              int
	allowedRatios                []string
	allowedResolutions           []string
	rejectExplicitServiceTier    bool
	enforceModelResolutionSuffix bool
}

type cangyuanRequestProfile struct {
	modelAgnostic         bool
	sd5Dialect            bool
	supportsSeed          bool
	maximumPromptLength   int
	minimumDuration       int
	maximumDuration       int
	allowedRatios         []string
	allowedResolutions    []string
	maximumImages         int
	maximumVideos         int
	maximumAudios         int
	maximumReferenceTotal int
	maximumVideoAudio     int
	maximumVideoDuration  int
}

type protocolProfile struct {
	channelName                        string
	modelList                          []string
	ignoreUnsupportedOptionalARKFields bool
	ignoredARKFields                   map[string]struct{}
	allowEmbeddedMedia                 bool
	requirePublicHTTPMedia             bool
	singleFrameImagesAreReferences     bool
	useRoutingDurationDefault          bool
	submitPath                         string
	pollPath                           string
	contentType                        string
	requestDialect                     videoRequestDialect
	defaultDurationSeconds             int
	textRequest                        *textRequestProfile
	cangyuanRequest                    *cangyuanRequestProfile
	secureRequest                      *secureRequestProfile
	omegaRequest                       *omegaRequestProfile
	untypedImagesAreReferences         bool
	allowEmptyReferenceMediaRoles      bool
	allowAudioWithoutVisual            bool
}

func genericProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:    ChannelName,
		submitPath:     "/v1/video/generations",
		pollPath:       "/v1/video/generations/{task_id}",
		contentType:    "application/json",
		requestDialect: videoRequestDialectNewAPIGenerations,
	}
}

func lucenProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName: ChannelNameLucen,
		modelList: []string{
			"seedance-480p-5s", "seedance-480p-10s", "seedance-480p-15s",
			"seedance-720p-5s", "seedance-720p-10s", "seedance-720p-15s",
			"seedance-1080p-5s", "seedance-1080p-10s", "seedance-1080p-15s",
			"seedance-480p-token", "seedance-720p-token", "seedance-1080p-token",
		},
		ignoreUnsupportedOptionalARKFields: true,
		ignoredARKFields: map[string]struct{}{
			"callback_url":            {},
			"return_last_frame":       {},
			"priority":                {},
			"execution_expires_after": {},
		},
		allowEmbeddedMedia:        true,
		useRoutingDurationDefault: true,
		submitPath:                "/v1/video/generations",
		pollPath:                  "/v1/video/generations/{task_id}",
		contentType:               "application/json",
		requestDialect:            videoRequestDialectNewAPIGenerations,
	}
}

func megaByAIProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:                    ChannelNameMegaByAI,
		modelList:                      []string{"videos-standard", "videos-fast", "videos-mini"},
		submitPath:                     "/v1/videos",
		pollPath:                       "/v1/videos/{task_id}",
		contentType:                    "application/json",
		requestDialect:                 videoRequestDialectMegaReferenceArrays,
		requirePublicHTTPMedia:         true,
		singleFrameImagesAreReferences: true,
		defaultDurationSeconds:         5,
	}
}

func cangyuanProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:                ChannelNameCangyuan,
		submitPath:                 "/v1/videos",
		pollPath:                   "/v1/videos/{task_id}",
		contentType:                "application/json",
		requestDialect:             videoRequestDialectCangyuanMedia,
		allowEmbeddedMedia:         true,
		requirePublicHTTPMedia:     true,
		untypedImagesAreReferences: true,
		cangyuanRequest: &cangyuanRequestProfile{
			minimumDuration:       4,
			maximumDuration:       15,
			allowedRatios:         []string{"16:9", "9:16", "1:1", "21:9", "3:4", "4:3"},
			allowedResolutions:    []string{"480p", "720p"},
			maximumImages:         4,
			maximumVideos:         3,
			maximumAudios:         1,
			maximumReferenceTotal: 8,
			maximumVideoDuration:  15,
		},
	}
}

func cangyuanGenericRequestProfile() cangyuanRequestProfile {
	profile := cangyuanRequestProfileForModel("")
	profile.modelAgnostic = true
	profile.supportsSeed = true
	profile.maximumImages = 9
	profile.maximumVideos = 3
	profile.maximumAudios = 3
	profile.maximumReferenceTotal = 12
	profile.maximumVideoAudio = 0
	profile.maximumPromptLength = 5000
	return profile
}

func cangyuanRequestProfileForModel(modelName string) cangyuanRequestProfile {
	profile := cangyuanProtocolProfile().cangyuanRequest
	if profile == nil {
		return cangyuanRequestProfile{}
	}
	result := *profile
	result.maximumPromptLength = 5000
	result.supportsSeed = false
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelName)), "sd5-seedance-") {
		result.sd5Dialect = true
		result.supportsSeed = true
		result.maximumPromptLength = 1200
		result.maximumImages = 9
		result.maximumAudios = 3
		result.maximumReferenceTotal = 12
		result.maximumVideoAudio = 3
	}
	return result
}

func paipuProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:                   ChannelNamePaipu,
		modelList:                     []string{},
		submitPath:                    "/v1/videos",
		pollPath:                      "/v1/videos/{task_id}",
		contentType:                   "application/json",
		requestDialect:                videoRequestDialectPaipuMediaArrays,
		allowEmbeddedMedia:            true,
		requirePublicHTTPMedia:        true,
		untypedImagesAreReferences:    true,
		allowEmptyReferenceMediaRoles: true,
		allowAudioWithoutVisual:       true,
	}
}

var secureModels = []string{"video-2.0-fast", "video-2.0-mini", "video-2.0-pro"}

func secureProtocolProfile(group dto.SecureVideoGroup) (protocolProfile, error) {
	profile := protocolProfile{
		channelName:            ChannelNameSecure,
		modelList:              append([]string(nil), secureModels...),
		requirePublicHTTPMedia: true,
		secureRequest:          &secureRequestProfile{group: group},
	}
	switch group {
	case dto.SecureVideoGroupDiscount:
		profile.submitPath = "/api/generate-video"
		profile.pollPath = "/api/task/{task_id}"
		profile.requestDialect = videoRequestDialectSecureDiscount
	case dto.SecureVideoGroupOverseas:
		profile.submitPath = "/api/generate-video"
		profile.pollPath = "/api/task/{task_id}"
		profile.requestDialect = videoRequestDialectSecureOverseas
	case dto.SecureVideoGroupEnterprise:
		profile.submitPath = "/v1/videos"
		profile.pollPath = "/v1/videos/{task_id}"
		profile.contentType = "application/json"
		profile.requestDialect = videoRequestDialectSecureEnterprise
	default:
		return protocolProfile{}, fmt.Errorf("invalid secure_video_group: %s", group)
	}
	return profile.normalized(), nil
}

func omegaAIProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName: ChannelNameOmegaAI,
		modelList: []string{
			"klsdpro2-720p",
			"seedance-v2-720p",
			"dola-seedance-2.0",
			"lingjing-video-v1",
			"db-ai-video-v1",
		},
		submitPath:             "/v1/media/generate",
		pollPath:               "/v1/tasks/{task_id}",
		contentType:            "application/json",
		requestDialect:         videoRequestDialectOmegaMediaArrays,
		requirePublicHTTPMedia: true,
		omegaRequest: &omegaRequestProfile{
			MaxImages: 9,
			MaxVideos: 3,
			MaxAudios: 3,
		},
	}
}

func fourSTokenProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:            ChannelNameFourSToken,
		modelList:              []string{},
		submitPath:             "/v1/videos",
		pollPath:               "/v1/videos/{task_id}",
		contentType:            "application/json",
		requestDialect:         videoRequestDialectFourSToken,
		requirePublicHTTPMedia: true,
	}
}

func eightYesProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:                    ChannelNameEightYes,
		modelList:                      []string{},
		submitPath:                     "/v1/videos",
		pollPath:                       "/v1/videos/{task_id}",
		contentType:                    "application/json",
		requestDialect:                 videoRequestDialectEightYes,
		requirePublicHTTPMedia:         true,
		singleFrameImagesAreReferences: true,
		defaultDurationSeconds:         5,
	}
}

func z5apiProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:             ChannelNameZ5API,
		modelList:               []string{},
		submitPath:              "/v1/videos",
		pollPath:                "/v1/videos/{task_id}",
		contentType:             "application/json",
		requestDialect:          videoRequestDialectZ5APIMedia,
		requirePublicHTTPMedia:  true,
		allowAudioWithoutVisual: true,
	}
}

func zzoneProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:                   ChannelNameZZone,
		modelList:                     []string{},
		submitPath:                    "/v1/videos",
		pollPath:                      "/v1/videos/{task_id}",
		contentType:                   "application/json",
		requestDialect:                videoRequestDialectZZone,
		requirePublicHTTPMedia:        true,
		untypedImagesAreReferences:    true,
		allowEmptyReferenceMediaRoles: true,
		allowAudioWithoutVisual:       true,
	}
}

func mikotoProtocolProfile() protocolProfile {
	return protocolProfile{
		channelName:        ChannelNameMikoto,
		modelList:          []string{},
		submitPath:         "/v1/videos",
		pollPath:           "/v1/videos/{task_id}",
		contentType:        "application/json",
		requestDialect:     videoRequestDialectMikoto,
		allowEmbeddedMedia: true,
	}
}

func (p protocolProfile) normalized() protocolProfile {
	if p.submitPath == "" {
		p.submitPath = "/v1/video/generations"
	}
	if p.pollPath == "" {
		p.pollPath = "/v1/video/generations/{task_id}"
	}
	if p.contentType == "" {
		p.contentType = "application/json"
	}
	if p.requestDialect == "" {
		p.requestDialect = videoRequestDialectNewAPIGenerations
	}
	return p
}

func NewLucenTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: lucenProtocolProfile()}
}

func NewMegaByAITaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: megaByAIProtocolProfile()}
}

func NewCangyuanTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: cangyuanProtocolProfile()}
}

func NewPaipuTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: paipuProtocolProfile()}
}

func NewSecureTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: protocolProfile{
		channelName: ChannelNameSecure,
		modelList:   append([]string(nil), secureModels...),
	}}
}

func NewOmegaAITaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: omegaAIProtocolProfile()}
}

func NewFourSTokenTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: fourSTokenProtocolProfile()}
}

func NewEightYesTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: eightYesProtocolProfile()}
}

func NewZ5APITaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: z5apiProtocolProfile()}
}

func NewZZoneTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: zzoneProtocolProfile()}
}

func NewMikotoTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: mikotoProtocolProfile()}
}

func (a *TaskAdaptor) activeProfile() protocolProfile {
	if a == nil || a.profile.channelName == "" {
		return genericProtocolProfile()
	}
	return a.profile.normalized()
}

func selectProtocolProfile(profiles []protocolProfile) protocolProfile {
	if len(profiles) == 0 || profiles[0].channelName == "" {
		return genericProtocolProfile()
	}
	return profiles[0].normalized()
}

func (a *TaskAdaptor) GetModelList() []string {
	return append([]string(nil), a.activeProfile().modelList...)
}

func (a *TaskAdaptor) GetChannelName() string {
	return a.activeProfile().channelName
}
