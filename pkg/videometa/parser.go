package videometa

import (
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/abema/go-mp4"
)

var (
	ErrUnsupportedContainer = errors.New("unsupported MP4/MOV container")
	ErrInvalidVideoTrack    = errors.New("invalid video track")
)

type parsedVideoTrack struct {
	durationMS   int64
	width        int
	height       int
	frameRateNum int64
	frameRateDen int64
}

func Parse(reader io.ReadSeeker, contentLength int64) (Metadata, error) {
	if reader == nil {
		return Metadata{}, fmt.Errorf("%w: reader is required", ErrUnsupportedContainer)
	}
	if contentLength < 0 {
		return Metadata{}, fmt.Errorf("%w: content length cannot be negative", ErrUnsupportedContainer)
	}
	if contentLength > MaxVideoBytes {
		return Metadata{}, newServiceError(ErrorMediaTooLarge, 0, errors.New("content length exceeds limit"))
	}

	container, err := containerType(reader)
	if err != nil {
		return Metadata{}, err
	}
	trackBoxes, err := mp4.ExtractBox(reader, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak()})
	if err != nil {
		return Metadata{}, fmt.Errorf("%w: %v", ErrUnsupportedContainer, err)
	}

	var selected *parsedVideoTrack
	for _, trackBox := range trackBoxes {
		track, trackErr := parseVideoTrack(reader, trackBox)
		if trackErr != nil {
			if errors.Is(trackErr, ErrInvalidVideoTrack) {
				continue
			}
			return Metadata{}, trackErr
		}
		if track == nil {
			continue
		}
		if selected == nil || int64(track.width)*int64(track.height) > int64(selected.width)*int64(selected.height) {
			selected = track
		}
	}
	if selected == nil {
		return Metadata{}, ErrInvalidVideoTrack
	}

	metadata := Metadata{
		DurationMS:    selected.durationMS,
		Width:         selected.width,
		Height:        selected.height,
		FrameRateNum:  selected.frameRateNum,
		FrameRateDen:  selected.frameRateDen,
		Container:     container,
		ContentLength: contentLength,
	}
	if err := metadata.Validate(); err != nil {
		return Metadata{}, fmt.Errorf("%w: %v", ErrInvalidVideoTrack, err)
	}
	return metadata, nil
}

func containerType(reader io.ReadSeeker) (string, error) {
	boxes, err := mp4.ExtractBoxWithPayload(reader, nil, mp4.BoxPath{mp4.BoxTypeFtyp()})
	if err != nil || len(boxes) == 0 {
		return "", ErrUnsupportedContainer
	}
	for _, box := range boxes {
		fileType, ok := box.Payload.(*mp4.Ftyp)
		if !ok {
			return "", ErrUnsupportedContainer
		}
		if string(fileType.MajorBrand[:]) == "qt  " {
			return "mov", nil
		}
		for _, compatible := range fileType.CompatibleBrands {
			if string(compatible.CompatibleBrand[:]) == "qt  " {
				return "mov", nil
			}
		}
	}
	return "mp4", nil
}

func parseVideoTrack(reader io.ReadSeeker, trackBox *mp4.BoxInfo) (*parsedVideoTrack, error) {
	boxes, err := mp4.ExtractBoxesWithPayload(reader, trackBox, []mp4.BoxPath{
		{mp4.BoxTypeTkhd()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMdhd()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeHdlr()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStts()},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidVideoTrack, err)
	}

	var trackHeader *mp4.Tkhd
	var mediaHeader *mp4.Mdhd
	var handler *mp4.Hdlr
	var timing *mp4.Stts
	for _, box := range boxes {
		switch box.Info.Type {
		case mp4.BoxTypeTkhd():
			trackHeader, _ = box.Payload.(*mp4.Tkhd)
		case mp4.BoxTypeMdhd():
			mediaHeader, _ = box.Payload.(*mp4.Mdhd)
		case mp4.BoxTypeHdlr():
			handler, _ = box.Payload.(*mp4.Hdlr)
		case mp4.BoxTypeStts():
			timing, _ = box.Payload.(*mp4.Stts)
		}
	}
	if handler == nil || handler.HandlerType != [4]byte{'v', 'i', 'd', 'e'} {
		return nil, nil
	}
	if trackHeader == nil || mediaHeader == nil || timing == nil {
		return nil, ErrInvalidVideoTrack
	}

	width := int(trackHeader.GetWidthInt())
	height := int(trackHeader.GetHeightInt())
	durationMS, err := durationMilliseconds(mediaHeader.GetDuration(), mediaHeader.Timescale)
	if err != nil {
		return nil, err
	}
	frameRateNum, frameRateDen, err := frameRate(timing, mediaHeader.Timescale)
	if err != nil {
		return nil, err
	}
	return &parsedVideoTrack{
		durationMS:   durationMS,
		width:        width,
		height:       height,
		frameRateNum: frameRateNum,
		frameRateDen: frameRateDen,
	}, nil
}

func durationMilliseconds(duration uint64, timescale uint32) (int64, error) {
	if duration == 0 || timescale == 0 {
		return 0, ErrInvalidVideoTrack
	}
	wholeSeconds := duration / uint64(timescale)
	if wholeSeconds > math.MaxInt64/1_000 {
		return 0, ErrInvalidVideoTrack
	}
	milliseconds := wholeSeconds * 1_000
	remainder := duration % uint64(timescale)
	fraction := (remainder*1_000 + uint64(timescale) - 1) / uint64(timescale)
	if milliseconds > math.MaxInt64-fraction {
		return 0, ErrInvalidVideoTrack
	}
	return int64(milliseconds + fraction), nil
}

func frameRate(timing *mp4.Stts, timescale uint32) (int64, int64, error) {
	if timing == nil || timescale == 0 || len(timing.Entries) == 0 {
		return 0, 0, ErrInvalidVideoTrack
	}
	var samples uint64
	var units uint64
	for _, entry := range timing.Entries {
		if entry.SampleCount == 0 || entry.SampleDelta == 0 {
			return 0, 0, ErrInvalidVideoTrack
		}
		entrySamples := uint64(entry.SampleCount)
		entryUnits := entrySamples * uint64(entry.SampleDelta)
		if samples > math.MaxUint64-entrySamples || units > math.MaxUint64-entryUnits {
			return 0, 0, ErrInvalidVideoTrack
		}
		samples += entrySamples
		units += entryUnits
	}
	if samples > math.MaxUint64/uint64(timescale) {
		return 0, 0, ErrInvalidVideoTrack
	}
	numerator := samples * uint64(timescale)
	divisor := greatestCommonDivisor(numerator, units)
	numerator /= divisor
	units /= divisor
	if numerator > math.MaxInt64 || units > math.MaxInt64 {
		return 0, 0, ErrInvalidVideoTrack
	}
	return int64(numerator), int64(units), nil
}

func greatestCommonDivisor(left, right uint64) uint64 {
	for right != 0 {
		left, right = right, left%right
	}
	return left
}
