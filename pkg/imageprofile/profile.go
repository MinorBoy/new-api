// Package imageprofile exposes the built-in image protocol contract.
//
// The relaykit module owns the concrete definitions so both modules can use
// the contract without introducing a module dependency cycle.
package imageprofile

import "github.com/QuantumNous/new-api/relaykit/imageprofile"

const (
	MaxImageN           = imageprofile.MaxImageN
	OpenAIImagesProfile = imageprofile.OpenAIImagesProfile
	OpenAIImagesVersion = imageprofile.OpenAIImagesVersion

	EndpointGenerations Endpoint = imageprofile.EndpointGenerations
	EndpointEdits       Endpoint = imageprofile.EndpointEdits

	CompatibilityUntested = imageprofile.CompatibilityUntested
	CompatibilityPassed   = imageprofile.CompatibilityPassed
	CompatibilityFailed   = imageprofile.CompatibilityFailed
	StatusUntested        = imageprofile.StatusUntested
	StatusPassed          = imageprofile.StatusPassed
	StatusFailed          = imageprofile.StatusFailed
)

type Endpoint = imageprofile.Endpoint
type Capability = imageprofile.Capability
type Profile = imageprofile.Profile
type ModelCapabilities = imageprofile.ModelCapabilities
type CompatibilityStatus = imageprofile.CompatibilityStatus
type Compatibility = imageprofile.Compatibility
type Binding = imageprofile.Binding

var Lookup = imageprofile.Lookup
