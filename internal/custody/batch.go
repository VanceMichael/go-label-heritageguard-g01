package custody

import (
	"context"
	"fmt"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type Item struct {
	LoanID string `json:"loan_id"`
	Seal   string `json:"seal"`
}

type Result struct {
	LoanID string `json:"loan_id"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

type Processor interface {
	ProcessGroup(context.Context, []Item) error
}

func ProcessGroups(ctx context.Context, groups [][]Item, processor Processor) ([]Result, error) {
	if len(groups) == 0 {
		return nil, domain.FieldError{Field: "groups", Message: "at least one atomic group is required"}
	}
	var results []Result
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		if len(group) == 0 {
			continue
		}
		err := processor.ProcessGroup(ctx, append([]Item(nil), group...))
		for _, item := range group {
			result := Result{LoanID: item.LoanID, OK: err == nil}
			if err != nil {
				result.Error = fmt.Sprintf("atomic group rejected: %v", err)
			}
			results = append(results, result)
		}
	}
	return results, nil
}
