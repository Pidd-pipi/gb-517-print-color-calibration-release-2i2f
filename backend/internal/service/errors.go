package service

import "errors"

var (
	ErrInvalidTransition = errors.New("requested status transition is not allowed")
	ErrInvalidInput      = errors.New("business input validation failed")
	ErrUnauthorized      = errors.New("invalid username or password")
	ErrInactiveUser      = errors.New("user account is inactive")
	ErrForbidden         = errors.New("role is not permitted for this operation")
	ErrLocked            = errors.New("resolved record is immutable")
	ErrProofGateBlocked  = errors.New("proof gate has not passed for this batch")
	ErrToleranceMissing  = errors.New("batch colour tolerance must be greater than zero before proof review")
	ErrProofNotLinked    = errors.New("proof must be linked to a print run before review")
	ErrRunNotGateable    = errors.New("linked batch is not in the proofing or hold state")
)
