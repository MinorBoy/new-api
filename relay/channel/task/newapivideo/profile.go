package newapivideo

import relaycommon "github.com/QuantumNous/new-api/relay/common"

const ChannelNameLucen = "Lucen"

const ChannelNameMegaByAI = "MegaByAI"

const ChannelNameCangyuan = "Cangyuan"

type videoRequestDialect string

const (
	videoRequestDialectNewAPIGenerations   videoRequestDialect = "newapi_generations"
	videoRequestDialectMegaReferenceArrays videoRequestDialect = "mega_reference_arrays"
	videoRequestDialectTextJSON            videoRequestDialect = "text_json"
)

type textRequestProfile struct {
	ratioField                string
	minimumDuration           int
	maximumDuration           int
	allowedRatios             []string
	allowedResolutions        []string
	rejectExplicitServiceTier bool
}

type protocolProfile struct {
	channelName                        string
	modelList                          []string
	ignoreUnsupportedOptionalARKFields bool
	ignoredARKFields                   map[string]struct{}
	allowEmbeddedMedia                 bool
	requirePublicHTTPMedia             bool
	useRoutingDurationDefault          bool
	submitPath                         string
	pollPath                           string
	contentType                        string
	requestDialect                     videoRequestDialect
	defaultDurationSeconds             int
	textRequest                        *textRequestProfile
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
		channelName:            ChannelNameMegaByAI,
		modelList:              []string{"videos-standard", "videos-fast", "videos-mini"},
		submitPath:             "/v1/videos",
		pollPath:               "/v1/videos/{task_id}",
		contentType:            "application/json",
		requestDialect:         videoRequestDialectMegaReferenceArrays,
		requirePublicHTTPMedia: true,
		defaultDurationSeconds: 5,
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
