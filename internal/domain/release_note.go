package domain

import (
	"fmt"
	"strings"
)

// ReleaseNoteType identifies the user-facing category of a release note.
type ReleaseNoteType string

const (
	ReleaseNoteTypeBreaking  ReleaseNoteType = "breaking"
	ReleaseNoteTypeFeature   ReleaseNoteType = "feature"
	ReleaseNoteTypeFix       ReleaseNoteType = "fix"
	ReleaseNoteTypeHighlight ReleaseNoteType = "highlight"
)

var orderedReleaseNoteTypes = []ReleaseNoteType{
	ReleaseNoteTypeBreaking,
	ReleaseNoteTypeFeature,
	ReleaseNoteTypeFix,
	ReleaseNoteTypeHighlight,
}

// ReleaseNote stores one custom release note entry collected from disk.
type ReleaseNote struct {
	Title      string
	Type       ReleaseNoteType
	Body       string
	SourcePath string
}

// ReleaseNotesCollection stores the rendered release-note inputs collected for a release.
type ReleaseNotesCollection struct {
	Notes    []ReleaseNote
	Warnings []string
}

// ParseReleaseNoteType validates and normalizes a release note type value.
func ParseReleaseNoteType(value string) (ReleaseNoteType, error) {
	normalized := ReleaseNoteType(strings.TrimSpace(strings.ToLower(value)))
	switch normalized {
	case ReleaseNoteTypeBreaking, ReleaseNoteTypeFeature, ReleaseNoteTypeFix, ReleaseNoteTypeHighlight:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid release note type: %s", value)
	}
}

// OrderedReleaseNoteTypes returns the rendering order for release note groups.
func OrderedReleaseNoteTypes() []ReleaseNoteType {
	return append([]ReleaseNoteType(nil), orderedReleaseNoteTypes...)
}

// Heading returns the markdown section heading for the note type.
func (t ReleaseNoteType) Heading() string {
	switch t {
	case ReleaseNoteTypeBreaking:
		return "#### Breaking Changes"
	case ReleaseNoteTypeFeature:
		return "#### Features"
	case ReleaseNoteTypeFix:
		return "#### Fixes"
	case ReleaseNoteTypeHighlight:
		return "#### Highlights"
	default:
		return "#### Other"
	}
}

// RenderMarkdown renders the collected release notes as markdown for PR bodies and release notes files.
func (c ReleaseNotesCollection) RenderMarkdown() string {
	if len(c.Notes) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("### Release Notes")
	currentType := ReleaseNoteType("")
	for index, note := range c.Notes {
		if note.Type != currentType {
			builder.WriteString("\n\n")
			builder.WriteString(note.Type.Heading())
			currentType = note.Type
		}
		builder.WriteString("\n\n##### ")
		builder.WriteString(note.Title)
		builder.WriteString("\n")
		builder.WriteString(strings.TrimSpace(note.Body))
		if index == len(c.Notes)-1 {
			continue
		}
	}
	return strings.TrimSpace(builder.String())
}
