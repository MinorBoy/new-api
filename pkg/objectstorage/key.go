package objectstorage

import (
	"fmt"
	"mime"
	"net/url"
	"path"
	"strings"
	"unicode"
)

func BuildVideoObjectKey(originModelName, publicTaskID, contentType, sourceURL string) (string, error) {
	modelSegment := sanitizeSegment(originModelName)
	taskSegment := sanitizeSegment(publicTaskID)
	if modelSegment == "" || taskSegment == "" {
		return "", fmt.Errorf("video object key requires model and task identifiers")
	}
	extension := videoExtension(contentType, sourceURL)
	return modelSegment + "/" + taskSegment + extension, nil
}

func sanitizeSegment(value string) string {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), ".-_ ")
}

func videoExtension(contentType, sourceURL string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	case "video/x-matroska":
		return ".mkv"
	case "video/mpeg":
		return ".mpeg"
	}
	if extensions, err := mime.ExtensionsByType(contentType); err == nil {
		for _, extension := range extensions {
			if isVideoExtension(extension) {
				return strings.ToLower(extension)
			}
		}
	}
	if parsed, err := url.Parse(strings.TrimSpace(sourceURL)); err == nil {
		extension := strings.ToLower(path.Ext(parsed.Path))
		if isVideoExtension(extension) {
			return extension
		}
	}
	return ".mp4"
}

func isVideoExtension(extension string) bool {
	switch strings.ToLower(extension) {
	case ".mp4", ".webm", ".mov", ".mkv", ".mpeg", ".mpg", ".m4v", ".avi":
		return true
	default:
		return false
	}
}
