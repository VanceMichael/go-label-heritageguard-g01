package exhibition

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type assessmentJobPayload struct {
	DisplayCaseID    string `json:"display_case_id"`
	TriggerReadingID string `json:"trigger_reading_id"`
	Window           string `json:"window"`
}

func (s *Service) ProcessAssessmentJob(ctx context.Context, job domain.WorkerJob) error {
	if job.Kind != EnvironmentAssessmentJob {
		return domain.FieldError{Field: "job.kind", Message: "unsupported exhibition job"}
	}
	var payload assessmentJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("decode environment assessment job: %w", err)
	}
	window, err := time.ParseDuration(payload.Window)
	if err != nil {
		return fmt.Errorf("parse environment assessment window: %w", err)
	}
	assessment, err := s.assessForTenant(ctx, job.TenantID, payload.DisplayCaseID, window)
	if err != nil {
		return fmt.Errorf("run environment assessment: %w", err)
	}
	if assessment.Ready {
		return nil
	}
	windowKey := assessment.WindowEnd.UTC().Truncate(window).Format(time.RFC3339)
	_, err = s.OpenIncident(ctx, job.TenantID, IncidentInput{
		DisplayCaseID: payload.DisplayCaseID,
		WindowKey:     windowKey,
		Kind:          "environment_threshold",
		Summary:       summarizeAssessment(assessment),
	})
	if err != nil {
		return fmt.Errorf("open environment incident from assessment: %w", err)
	}
	return nil
}

func summarizeAssessment(assessment domain.EnvironmentAssessment) string {
	if len(assessment.Reasons) == 0 {
		return "environment assessment did not satisfy exhibition policy"
	}
	result := "environment assessment failed: "
	for index, reason := range assessment.Reasons {
		if index != 0 {
			result += "; "
		}
		result += reason
	}
	return result
}
