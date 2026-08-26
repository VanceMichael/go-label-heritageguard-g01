package custody

import (
	"context"
	"errors"
	"testing"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

type recordingProcessor struct {
	groups [][]Item
	err    error
}

func (p *recordingProcessor) ProcessGroup(_ context.Context, items []Item) error {
	p.groups = append(p.groups, append([]Item(nil), items...))
	return p.err
}

func TestProcessGroupsPreservesAtomicGroupResults(t *testing.T) {
	processor := &recordingProcessor{}
	results, err := ProcessGroups(context.Background(), [][]Item{
		{{LoanID: "loan-1"}, {LoanID: "loan-2"}},
		{{LoanID: "loan-3"}},
	}, processor)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || len(processor.groups) != 2 {
		t.Fatalf("unexpected groups/results: %#v %#v", processor.groups, results)
	}
	for _, result := range results {
		if !result.OK || result.Error != "" {
			t.Fatalf("unexpected successful result: %#v", result)
		}
	}
	processor.err = errors.New("seal mismatch")
	results, err = ProcessGroups(context.Background(), [][]Item{{{LoanID: "loan-4"}, {LoanID: "loan-5"}}}, processor)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.OK || result.Error == "" {
			t.Fatalf("expected rejected group result: %#v", result)
		}
	}
}

func TestProcessGroupsValidationAndCancellation(t *testing.T) {
	processor := &recordingProcessor{}
	if _, err := ProcessGroups(context.Background(), nil, processor); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("expected invalid groups, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results, err := ProcessGroups(ctx, [][]Item{{{LoanID: "loan-1"}}}, processor)
	if !errors.Is(err, context.Canceled) || len(results) != 0 {
		t.Fatalf("expected cancellation before processing, got %#v %v", results, err)
	}
}
