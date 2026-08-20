package charging

import (
	"context"
	"fmt"
	"time"
	"voltforge/internal/auth"
)

type Repository interface {
	CreateProduct(context.Context, *Product) error
	GetProduct(context.Context, string) (*Product, error)
	ListProducts(context.Context, int, int) ([]Product, int, error)
	CreateIssue(context.Context, *Issue) error
	GetIssue(context.Context, string) (*Issue, error)
	TransitionIssue(context.Context, *Issue, IssueState, string, string) error
	CreateChargeTest(context.Context, *ChargeTest) error
	ListOpenIssues(context.Context, string, int, int) ([]Issue, int, error)
}

type Service struct {
	repo Repository
	auth *auth.Service
	now  func() time.Time
}

func NewService(repo Repository, authService *auth.Service, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, auth: authService, now: now}
}

func (s *Service) CreateProduct(ctx context.Context, token string, product Product) (Product, error) {
	if _, err := s.auth.RequireRole(ctx, token, auth.RoleLabReviewer, auth.RoleAuditor); err != nil {
		return Product{}, err
	}
	if product.ID == "" || product.Name == "" || product.VendorID == "" {
		return Product{}, fmt.Errorf("product fields: %w", ErrInvalidState)
	}
	if product.SupportedProtocols == "" {
		product.SupportedProtocols = "USB-PD"
	}
	if product.MaxPowerWatts <= 0 {
		product.MaxPowerWatts = 65
	}
	if product.PortCount <= 0 {
		product.PortCount = 1
	}
	if product.ThermalLimitC <= 0 {
		product.ThermalLimitC = 55
	}
	if product.CellCount <= 0 {
		product.CellCount = 1
	}
	product.Status = ProductActive
	product.CreatedAt = s.now()
	product.UpdatedAt = product.CreatedAt
	product.Version = 1
	if err := s.repo.CreateProduct(ctx, &product); err != nil {
		return Product{}, err
	}
	return product, nil
}

func (s *Service) ReportIssue(ctx context.Context, token string, issue Issue) (Issue, error) {
	session, err := s.auth.RequireRole(ctx, token, auth.RoleTestEngineer, auth.RoleAuditor)
	if err != nil {
		return Issue{}, err
	}
	if _, err := s.repo.GetProduct(ctx, issue.ProductID); err != nil {
		return Issue{}, err
	}
	if issue.ID == "" || issue.Kind == "" || issue.Description == "" {
		return Issue{}, fmt.Errorf("issue fields: %w", ErrInvalidState)
	}
	issue.SubmittedBy = session.UserID
	issue.State = IssueOpen
	issue.CreatedAt = s.now()
	issue.UpdatedAt = issue.CreatedAt
	issue.Version = 1
	if issue.ReviewDueAt.IsZero() {
		issue.ReviewDueAt = s.now().Add(48 * time.Hour)
	}
	if err := s.repo.CreateIssue(ctx, &issue); err != nil {
		return Issue{}, err
	}
	return issue, nil
}

func (s *Service) AssignIssue(ctx context.Context, token, id, vendorengineerID string) error {
	if _, err := s.auth.RequireRole(ctx, token, auth.RoleLabReviewer, auth.RoleAuditor); err != nil {
		return err
	}
	h, err := s.repo.GetIssue(ctx, id)
	if err != nil {
		return err
	}
	if !h.CanTransition(IssueAssigned) {
		return ErrInvalidState
	}
	return s.repo.TransitionIssue(ctx, h, IssueAssigned, vendorengineerID, "")
}

func (s *Service) MitigateIssue(ctx context.Context, token, id, firmware_evidence string) error {
	session, err := s.auth.RequireRole(ctx, token, auth.RoleVendorEngineer)
	if err != nil {
		return err
	}
	if firmware_evidence == "" {
		return ErrMissingFirmwareEvidence
	}
	h, err := s.repo.GetIssue(ctx, id)
	if err != nil {
		return err
	}
	if h.VendorAssigneeID != session.UserID && h.VendorAssigneeID != "" {
		return auth.ErrForbidden
	}
	if !h.CanTransition(IssueMitigated) {
		return ErrInvalidState
	}
	return s.repo.TransitionIssue(ctx, h, IssueMitigated, session.UserID, firmware_evidence)
}

func (s *Service) CertifyIssue(ctx context.Context, token, id, firmware_evidence string) error {
	if _, err := s.auth.RequireRole(ctx, token, auth.RoleTestEngineer, auth.RoleLabReviewer); err != nil {
		return err
	}
	if firmware_evidence == "" {
		return ErrMissingFirmwareEvidence
	}
	h, err := s.repo.GetIssue(ctx, id)
	if err != nil {
		return err
	}
	if !h.CanTransition(IssueCertified) {
		return ErrInvalidState
	}
	return s.repo.TransitionIssue(ctx, h, IssueCertified, "", firmware_evidence)
}
