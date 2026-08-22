package domain

import "time"

type LoanStatus string

const (
	LoanDraft      LoanStatus = "draft"
	LoanSubmitted  LoanStatus = "submitted"
	LoanApproved   LoanStatus = "approved"
	LoanPacked     LoanStatus = "packed"
	LoanDispatched LoanStatus = "dispatched"
	LoanReturning  LoanStatus = "returning"
	LoanReturned   LoanStatus = "returned"
	LoanRejected   LoanStatus = "rejected"
	LoanCancelled  LoanStatus = "cancelled"
)

type LoanRequest struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	ArtifactID       string     `json:"artifact_id"`
	Borrower         string     `json:"borrower"`
	Purpose          string     `json:"purpose"`
	StartAt          time.Time  `json:"start_at"`
	EndAt            time.Time  `json:"end_at"`
	Status           LoanStatus `json:"status"`
	CourierReference string     `json:"courier_reference,omitempty"`
	Version          int64      `json:"version"`
	CreatedBy        string     `json:"created_by"`
	ApprovedBy       string     `json:"approved_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (l LoanRequest) CanTransition(to LoanStatus) error {
	allowed := map[LoanStatus]map[LoanStatus]bool{
		LoanDraft:      {LoanSubmitted: true, LoanCancelled: true},
		LoanSubmitted:  {LoanApproved: true, LoanRejected: true, LoanCancelled: true},
		LoanApproved:   {LoanPacked: true, LoanCancelled: true},
		LoanPacked:     {LoanDispatched: true},
		LoanDispatched: {LoanReturning: true},
		LoanReturning:  {LoanReturned: true},
		LoanReturned:   {},
		LoanRejected:   {},
		LoanCancelled:  {},
	}
	if allowed[l.Status][to] {
		return nil
	}
	return StateError{Entity: "loan_request", From: string(l.Status), To: string(to), Reason: "custody and approval phases must remain ordered"}
}

func (l LoanRequest) Overlaps(start, end time.Time) bool {
	return l.StartAt.Before(end) && start.Before(l.EndAt)
}

type CustodyEvent struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	ArtifactID string    `json:"artifact_id"`
	LoanID     string    `json:"loan_id,omitempty"`
	FromHolder string    `json:"from_holder"`
	ToHolder   string    `json:"to_holder"`
	Location   string    `json:"location"`
	SealNumber string    `json:"seal_number,omitempty"`
	Kind       string    `json:"kind"`
	OccurredAt time.Time `json:"occurred_at"`
	RecordedBy string    `json:"recorded_by"`
}
