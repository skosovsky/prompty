package prompty

import "slices"

// CaptureSchemaDigest associates source/translated contracts without raw schema
// values, endpoint credentials or profile provenance.
type CaptureSchemaDigest struct {
	Role             SchemaRole `json:"role"`
	Name             string     `json:"name,omitempty"`
	SourceDigest     string     `json:"source_digest"`
	TranslatedDigest string     `json:"translated_digest"`
}

// CaptureRecordMetadata describes observed fields, never inferred transport
// coverage. Provider response identity and HTTP request identity are distinct.
// It is carried inside the host-approved CaptureProjection.
type CaptureRecordMetadata struct {
	ProviderResponseID string                `json:"provider_response_id,omitempty"`
	RequestID          string                `json:"request_id,omitempty"`
	HTTPStatus         *int                  `json:"http_status,omitempty"`
	Usage              *Usage                `json:"usage,omitempty"`
	Outcome            Outcome               `json:"outcome,omitempty"`
	SchemaDigests      []CaptureSchemaDigest `json:"schema_digests,omitempty"`
}

// Validate rejects malformed known usage/outcome/status before sink delivery.
func (m CaptureRecordMetadata) Validate() error {
	if m.HTTPStatus != nil && (*m.HTTPStatus < 100 || *m.HTTPStatus > 599) {
		return ErrCaptureUnsupported
	}
	if m.Outcome != "" && !validOutcome(m.Outcome) {
		return ErrResponseOutcome
	}
	if m.Usage != nil {
		return m.Usage.Validate()
	}
	return nil
}

// CloneCaptureRecordMetadata isolates callback-owned pointers and collections.
func CloneCaptureRecordMetadata(m CaptureRecordMetadata) CaptureRecordMetadata {
	out := m
	out.SchemaDigests = slices.Clone(m.SchemaDigests)
	if m.HTTPStatus != nil {
		status := *m.HTTPStatus
		out.HTTPStatus = &status
	}
	if m.Usage != nil {
		usage := *m.Usage
		usage.Known = slices.Clone(m.Usage.Known)
		out.Usage = &usage
	}
	return out
}

// CaptureSchemaDigests projects immutable compilation associations, not schema
// content or provider scope. The host may omit these through its projector.
func CaptureSchemaDigests(reports []SchemaCompilationReport) []CaptureSchemaDigest {
	digests := make([]CaptureSchemaDigest, len(reports))
	for index, report := range reports {
		digests[index] = CaptureSchemaDigest{Role: report.Role, Name: report.Name,
			SourceDigest: report.SourceDigest, TranslatedDigest: report.TranslatedDigest}
	}
	return digests
}
