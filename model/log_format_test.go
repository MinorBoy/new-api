package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatUserLogsAssignsDisplayIDsAndClearsChannelNames(t *testing.T) {
	logs := []*Log{
		{Id: 41, ChannelName: "supplier-a"},
		{Id: 42, ChannelName: "supplier-b"},
	}

	formatUserLogs(logs, 10)

	require.Equal(t, 11, logs[0].Id)
	require.Equal(t, 12, logs[1].Id)
	require.Empty(t, logs[0].ChannelName)
	require.Empty(t, logs[1].ChannelName)
}
