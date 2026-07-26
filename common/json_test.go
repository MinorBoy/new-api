package common

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type strictJSONPayload struct {
	Name string `json:"name"`
}

func TestJsonRawMessageToString(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
		want string
	}{
		{
			name: "object",
			data: json.RawMessage(`{"city":"Paris","days":0,"strict":false}`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "string",
			data: json.RawMessage(`"{\"city\":\"Paris\",\"days\":0,\"strict\":false}"`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "null",
			data: json.RawMessage(`null`),
			want: "",
		},
		{
			name: "empty",
			data: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, JsonRawMessageToString(tt.data))
		})
	}
}

func TestDecodeJsonStrictRejectsUnknownFields(t *testing.T) {
	var payload strictJSONPayload
	err := DecodeJsonStrict(strings.NewReader(`{"name":"channel","extra":true}`), &payload)

	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

func TestDecodeJsonStrictRejectsTrailingJSONValue(t *testing.T) {
	var payload strictJSONPayload
	err := DecodeJsonStrict(strings.NewReader(`{"name":"channel"} {"name":"second"}`), &payload)

	require.Error(t, err)
}

func TestDecodeJsonStrictAcceptsSingleValidValue(t *testing.T) {
	var payload strictJSONPayload
	err := DecodeJsonStrict(strings.NewReader(`{"name":"channel"}`), &payload)

	require.NoError(t, err)
	require.Equal(t, "channel", payload.Name)
}
