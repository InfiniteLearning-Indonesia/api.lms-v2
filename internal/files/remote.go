package files

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/egress"
	"lmsv3/internal/platform/transaction"
)

func (s *Store) ImportRemote(ctx context.Context, token, key, requestID, rawURL string, input Upload) (Asset, bool, error) {
	if s.remote == nil || len(rawURL) < 10 || len(rawURL) > 2048 || strings.TrimSpace(rawURL) != rawURL {
		return Asset{}, false, ErrInvalid
	}
	input, err := normalizeMetadata(input)
	if err != nil || !idempotencyPattern.MatchString(key) || requestID == "" {
		return Asset{}, false, ErrInvalid
	}
	// Authorize before DNS or transport work so an outsider cannot use this
	// endpoint as a blind network oracle or consume the outbound budget.
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, resolveErr := s.resolve(ctx, tx, token)
		if resolveErr != nil {
			return resolveErr
		}
		return s.classContext(ctx, tx, actor, input.ClassID, "upload", input.Purpose, input.OwnerUserID)
	})
	if err != nil {
		return Asset{}, false, err
	}
	fetched, err := s.remote.Fetch(ctx, rawURL)
	if err != nil {
		switch {
		case errors.Is(err, egress.ErrPolicy), errors.Is(err, egress.ErrAddress), errors.Is(err, egress.ErrTooLarge):
			return Asset{}, false, ErrInvalid
		default:
			return Asset{}, false, ErrUnavailable
		}
	}
	if fetched.MediaType != input.DeclaredType {
		return Asset{}, false, ErrInvalid
	}
	input.Data = fetched.Data
	input.requestScope = rawURL
	return s.Upload(ctx, token, key, requestID, input)
}
