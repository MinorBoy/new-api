package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateRoleAssetURLRejectsUnsafeSources(t *testing.T) {
	tests := []string{
		"data:image/png;base64,AAAA",
		"file:///tmp/character.png",
		"asset://asset-project-one",
		"http://localhost/character.png",
		"http://sub.localhost/character.png",
		"http://127.0.0.1/character.png",
		"http://10.0.0.1/character.png",
		"http://172.16.0.1/character.png",
		"http://192.168.1.1/character.png",
		"http://[::1]/character.png",
	}

	for _, rawURL := range tests {
		t.Run(rawURL, func(t *testing.T) {
			assert.Error(t, ValidateRoleAssetURL(rawURL))
		})
	}
}

func TestValidateRoleAssetURLAllowsPublicHTTPS(t *testing.T) {
	assert.NoError(t, ValidateRoleAssetURL("https://8.8.8.8/character.png"))
}
