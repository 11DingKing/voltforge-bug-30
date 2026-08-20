package charging

import (
	"context"
	"testing"
	"time"
	"voltforge/internal/auth"
)

type fakeRepo struct {
	products map[string]*Product
	issues   map[string]*Issue
}

func (f *fakeRepo) CreateProduct(_ context.Context, s *Product) error {
	f.products[s.ID] = s
	return nil
}
func (f *fakeRepo) GetProduct(_ context.Context, id string) (*Product, error) {
	s, ok := f.products[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}
func (f *fakeRepo) ListProducts(context.Context, int, int) ([]Product, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) CreateIssue(_ context.Context, h *Issue) error { f.issues[h.ID] = h; return nil }
func (f *fakeRepo) GetIssue(_ context.Context, id string) (*Issue, error) {
	h, ok := f.issues[id]
	if !ok {
		return nil, ErrNotFound
	}
	return h, nil
}
func (f *fakeRepo) TransitionIssue(_ context.Context, h *Issue, next IssueState, assigned, firmware_evidence string) error {
	h.State = next
	h.VendorAssigneeID = assigned
	h.FirmwareEvidence = firmware_evidence
	return nil
}
func (f *fakeRepo) CreateChargeTest(context.Context, *ChargeTest) error { return nil }
func (f *fakeRepo) ListOpenIssues(context.Context, string, int, int) ([]Issue, int, error) {
	return nil, 0, nil
}

func TestIssueLifecycleRequiresFirmwareEvidence(t *testing.T) {
	repo := &fakeRepo{products: map[string]*Product{}, issues: map[string]*Issue{}}
	a := auth.New(time.Hour, time.Now)
	svc := NewService(repo, a, time.Now)
	ctx := context.Background()
	r, _ := a.Login(ctx, "labreviewer", "labreviewer-demo")
	o, _ := a.Login(ctx, "vendorengineer", "vendorengineer-demo")
	i, _ := a.Login(ctx, "testengineer", "testengineer-demo")
	if _, err := svc.CreateProduct(ctx, r.ID, Product{ID: "st-1", Name: "人民路站", VendorID: "u-vendorengineer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReportIssue(ctx, i.ID, Issue{ID: "hz-1", ProductID: "st-1", Kind: "expired_extinguisher", Description: "灭火器压力不足"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssignIssue(ctx, r.ID, "hz-1", o.UserID); err != nil {
		t.Fatal(err)
	}
	if err := svc.MitigateIssue(ctx, o.ID, "hz-1", ""); err != ErrMissingFirmwareEvidence {
		t.Fatalf("got %v", err)
	}
	if err := svc.MitigateIssue(ctx, o.ID, "hz-1", "photo://hz-1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CertifyIssue(ctx, i.ID, "hz-1", "check://hz-1"); err != nil {
		t.Fatal(err)
	}
}
