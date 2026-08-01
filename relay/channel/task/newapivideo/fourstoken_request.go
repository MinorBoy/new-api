package newapivideo

import "github.com/QuantumNous/new-api/common"

type fourSTokenRequest struct {
	Model         string       `json:"model"`
	Content       []arkContent `json:"content"`
	GenerateAudio *bool        `json:"generate_audio,omitempty"`
	Ratio         *string      `json:"ratio,omitempty"`
	Duration      *int         `json:"duration,omitempty"`
	Watermark     *bool        `json:"watermark,omitempty"`
	Resolution    *string      `json:"resolution,omitempty"`
	Seed          *int64       `json:"seed,omitempty"`
}

func validateFourSTokenRequest(request arkRequest, upstreamModel string) error {
	if err := validateARKSemantics(request, fourSTokenProtocolProfile()); err != nil {
		return err
	}
	if upstreamModel == "" {
		return nil
	}
	return nil
}

func buildFourSTokenRequest(request arkRequest, upstreamModel string) ([]byte, error) {
	if err := validateFourSTokenRequest(request, upstreamModel); err != nil {
		return nil, err
	}
	result := fourSTokenRequest{
		Model:         upstreamModel,
		Content:       append([]arkContent(nil), request.Content...),
		GenerateAudio: request.GenerateAudio,
		Ratio:         request.Ratio,
		Duration:      request.Duration,
		Watermark:     request.Watermark,
		Resolution:    request.Resolution,
	}
	if request.Seed != nil {
		seed, err := request.Seed.Int64()
		if err != nil {
			return nil, &arkRequestError{Code: "InvalidParameter.seed", Message: "seed must be an integer between -1 and 4294967295"}
		}
		result.Seed = &seed
	}
	return common.Marshal(result)
}
