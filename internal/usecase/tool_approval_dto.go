package usecase

type ResolveToolApprovalInput struct {
	Approved bool
	OptionID string
	Comment  string
	UserID   string
}
