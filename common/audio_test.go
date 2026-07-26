package common

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetAudioDurationHonorsCanceledContextBeforeParsing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GetAudioDuration(ctx, strings.NewReader("untrusted audio payload"), ".aac")
	require.ErrorIs(t, err, context.Canceled)
}

type cancelingReadSeeker struct {
	reader *strings.Reader
	cancel context.CancelFunc
}

func (r *cancelingReadSeeker) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	r.cancel()
	return n, err
}

func (r *cancelingReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return r.reader.Seek(offset, whence)
}

func TestGetAudioDurationStopsWhenContextCancelsDuringParsing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelingReadSeeker{reader: strings.NewReader(strings.Repeat("x", 4096)), cancel: cancel}

	_, err := GetAudioDuration(ctx, reader, ".aac")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
}
