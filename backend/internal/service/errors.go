package service

import "errors"

var (
	ErrInvalidTransition = errors.New("requested status transition is not allowed")
	ErrInvalidInput      = errors.New("business input validation failed")
	ErrUnauthorized      = errors.New("invalid username or password")
	ErrInactiveUser      = errors.New("user account is inactive")
	ErrForbidden         = errors.New("role is not permitted for this operation")
	ErrLocked            = errors.New("resolved record is immutable")
	ErrBatchNotLinked    = errors.New("proof is not linked to a print run")
	ErrBatchNotProofing  = errors.New("linked print run is not in proofing")
	ErrToleranceMissing  = errors.New("linked print run has no tolerance configured")
	ErrBatchHeld         = errors.New("linked print run is on hold after an out-of-tolerance proof")
)
