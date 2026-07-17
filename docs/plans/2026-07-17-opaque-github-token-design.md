# Opaque GitHub Token Validation

## Problem

GitHub Actions run `29551163720` fails before the dry-run orchestrator starts because `ValidateGitHubToken` requires installation tokens to match `ghs_` followed by exactly 36 alphanumeric characters. GitHub began issuing variable-length stateless installation tokens in the `ghs_APPID_JWT` format in April 2026, including the automatic Actions `GITHUB_TOKEN`.

## Design

GitHub credentials must be treated as opaque values. Configuration validation will require a non-empty token and reject whitespace or control characters, but it will not inspect prefixes, lengths, or internal encoding. GitHub remains the authority that determines whether a syntactically safe credential is authentic and authorized.

The workflow remains unchanged because `${{ secrets.GITHUB_TOKEN }}` is the supported automatic token source. Repository constructors continue using the shared validator, so the corrected contract applies consistently to configuration and GitHub clients.

## Error Handling

Empty tokens and tokens containing whitespace or control characters fail locally with specific validation errors. Opaque tokens that pass lexical validation but are invalid or unauthorized fail at the GitHub API boundary with the existing contextual errors.

## Testing

The invariant belongs to the config layer: GitHub credentials are opaque and must not be rejected because their provider-controlled format or length changes. The canonical `internal/config/config_test.go` suite will add focused coverage for the stateless installation-token shape, arbitrary opaque token values, empty input, and whitespace/control characters.
