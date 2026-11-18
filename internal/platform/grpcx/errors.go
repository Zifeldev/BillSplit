package grpcx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

// ToStatus maps domain errors to gRPC codes and hides internal details.
func ToStatus(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok && status.Code(err) != codes.Unknown {
		return err // already a gRPC error from a downstream service
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, "invalid email or password")
	case errors.Is(err, domain.ErrUnauthenticated):
		return status.Error(codes.Unauthenticated, "unauthenticated")
	case errors.Is(err, domain.ErrForbidden):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request timed out")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request cancelled")
	}
	logger.FromContext(ctx).Error("internal error", "err", err)
	return status.Error(codes.Internal, "internal error")
}

// FromStatus converts a downstream gRPC error back into a domain error,
// so services can use errors.Is regardless of where the error came from.
func FromStatus(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	var base error
	switch st.Code() {
	case codes.NotFound:
		base = domain.ErrNotFound
	case codes.AlreadyExists:
		base = domain.ErrAlreadyExists
	case codes.InvalidArgument:
		base = domain.ErrInvalidArgument
	case codes.PermissionDenied:
		base = domain.ErrForbidden
	case codes.FailedPrecondition:
		base = domain.ErrFailedPrecondition
	case codes.Unauthenticated:
		base = domain.ErrUnauthenticated
	default:
		return fmt.Errorf("downstream: %w", err)
	}
	return fmt.Errorf("%w: %s", base, st.Message())
}

func TS(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func TSPtr(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func FromTS(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.AsTime()
}
