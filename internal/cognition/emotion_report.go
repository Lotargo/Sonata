package cognition

import (
	"errors"
	"strings"
)

// EmotionReportContractVersion identifies the only affective report contract
// accepted by the mini MVP cognitive pipeline.
const EmotionReportContractVersion = "sonata-emotion-report-v1"

// ContractVersion returns the version shared by every allowed role projection.
// The version is attached to the Go type rather than repeated in every value,
// so one cognitive run cannot mix report schemas.
func (EmotionReport) ContractVersion() string {
	return EmotionReportContractVersion
}

func (report EmotionReport) Validate() error {
	if strings.TrimSpace(report.Text) == "" {
		return errors.New("emotion report text is required")
	}
	if report.StateVersion < 0 {
		return errors.New("emotion report state version cannot be negative")
	}
	return nil
}
