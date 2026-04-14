package usecase

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/compozy/releasepr/internal/domain"
)

// PreparePRBodyUseCase contains the logic for the prepare-pr-body command.
type PreparePRBodyUseCase struct {
}

func (uc *PreparePRBodyUseCase) validateMarkdownContent(fieldName, content string) error {
	switch {
	case strings.ContainsRune(content, '\x00'):
		return fmt.Errorf("%s contains invalid null byte", fieldName)
	default:
		return nil
	}
}

// Execute runs the use case.
func (uc *PreparePRBodyUseCase) Execute(_ context.Context, release *domain.Release) (string, error) {
	if release == nil {
		return "", fmt.Errorf("release cannot be nil")
	}
	if release.Version == nil {
		return "", fmt.Errorf("release version cannot be nil")
	}
	if err := uc.validateMarkdownContent("changelog", release.Changelog); err != nil {
		return "", err
	}
	if err := uc.validateMarkdownContent("release notes", release.ReleaseNotes); err != nil {
		return "", err
	}
	safeData := struct {
		Version      string
		Changelog    string
		ReleaseNotes string
	}{
		Version:      release.Version.String(),
		Changelog:    strings.TrimSpace(release.Changelog),
		ReleaseNotes: strings.TrimSpace(release.ReleaseNotes),
	}
	tmpl := template.New("pr-body")
	tmpl = tmpl.Option("missingkey=error")
	parsedTmpl, err := tmpl.Parse(prBodyTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse PR body template: %w", err)
	}
	var buf bytes.Buffer
	if err := parsedTmpl.Execute(&buf, safeData); err != nil {
		return "", fmt.Errorf("failed to execute PR body template: %w", err)
	}
	output := buf.String()
	if err := uc.validateMarkdownContent("pr body", output); err != nil {
		return "", fmt.Errorf("potential injection detected in PR body output")
	}
	return output, nil
}

const prBodyTemplate = `
## Release {{.Version}}

This PR prepares the release of version {{.Version}}.

### Changelog

{{.Changelog}}{{if .ReleaseNotes}}

{{.ReleaseNotes}}{{end}}
`
