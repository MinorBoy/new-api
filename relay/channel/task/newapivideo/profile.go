package newapivideo

const ChannelNameLucen = "Lucen"

type protocolProfile struct {
	channelName                        string
	modelList                          []string
	ignoreUnsupportedOptionalARKFields bool
	allowEmbeddedMedia                 bool
	useRoutingDurationDefault          bool
}

func genericProtocolProfile() protocolProfile {
	return protocolProfile{channelName: ChannelName}
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
		allowEmbeddedMedia:                 true,
		useRoutingDurationDefault:          true,
	}
}

func NewLucenTaskAdaptor() *TaskAdaptor {
	return &TaskAdaptor{profile: lucenProtocolProfile()}
}

func (a *TaskAdaptor) activeProfile() protocolProfile {
	if a == nil || a.profile.channelName == "" {
		return genericProtocolProfile()
	}
	return a.profile
}

func selectProtocolProfile(profiles []protocolProfile) protocolProfile {
	if len(profiles) == 0 || profiles[0].channelName == "" {
		return genericProtocolProfile()
	}
	return profiles[0]
}

func (a *TaskAdaptor) GetModelList() []string {
	return append([]string(nil), a.activeProfile().modelList...)
}

func (a *TaskAdaptor) GetChannelName() string {
	return a.activeProfile().channelName
}
