package newapivideo

import (
	"fmt"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
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

type videoRequestDialect string

const (
	videoRequestDialectNewAPIGenerations   videoRequestDialect = "newapi_generations"
	videoRequestDialectMegaReferenceArrays videoRequestDialect = "mega_reference_arrays"
	videoRequestDialectTextJSON            videoRequestDialect = "text_json"
	videoRequestDialectSecureDiscount      videoRequestDialect = "secure_discount"
	videoRequestDialectSecureOverseas      videoRequestDialect = "secure_overseas"
	videoRequestDialectSecureEnterprise    videoRequestDialect = "secure_enterprise"
	videoRequestDialectOmegaMediaArrays    videoRequestDialect = "omega_media_arrays"
	videoRequestDialectFourSToken          videoRequestDialect = "fourstoken"
	videoRequestDialectEightYes            videoRequestDialect = "eightyes"
	videoRequestDialectPaipuMediaArrays    videoRequestDialect = "paipu_media_arrays"
	videoRequestDialectZ5APIMedia          videoRequestDialect = "z5api_media"
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
		channelName:    ChannelNameCangyuan,
		modelList:      []string{"seedance-2.0-720p"},
		submitPath:     "/v1/videos",
		pollPath:       "/v1/videos/{task_id}",
		contentType:    "application/json",
		requestDialect: videoRequestDialectTextJSON,
		textRequest: &textRequestProfile{
			ratioField:                "aspect_ratio",
			minimumDuration:           1,
			maximumDuration:           relaycommon.MaxTaskDurationSeconds,
			rejectExplicitServiceTier: true,
		},
	}
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
