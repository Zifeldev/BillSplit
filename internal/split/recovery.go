package split

import (
	"context"
	"log/slog"
	"time"

	"github.com/Zifeldev/Billsplit/internal/domain"
	"github.com/Zifeldev/Billsplit/internal/platform/logger"
)

// Self-healing for receipts stuck in PENDING (OCR event lost to a crash, a
// message parked in the DLQ while a dependency was restarting, ...).
// The OCR worker is idempotent — it skips receipts that are no longer PENDING —
// so re-queueing is always safe.
const (
	pendingRetryAfter = 2 * time.Minute // re-queue OCR if nothing happened for this long
	pendingGiveUp     = time.Hour       // then stop retrying and let the user enter items
	recoveryInterval  = time.Minute
	recoveryBatch     = 100
	giveUpMessage     = "couldn't read the receipt in time — enter the items manually"
)

// RunPendingRecovery re-queues stale PENDING receipts until ctx is cancelled.
func (s *Service) RunPendingRecovery(ctx context.Context, log *slog.Logger) error {
	log = log.With("component", "pending-recovery")
	ctx = logger.WithContext(ctx, log)
	t := time.NewTicker(recoveryInterval)
	defer t.Stop()
	for {
		if requeued, failed, err := s.RecoverPending(ctx, time.Now()); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Warn("pending receipts recovery failed", "err", err)
		} else if requeued+int(failed) > 0 {
			log.Info("pending receipts recovered", "requeued", requeued, "failed", failed)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// RecoverPending runs one pass: give up on very old receipts, re-queue the rest.
func (s *Service) RecoverPending(ctx context.Context, now time.Time) (requeued int, failed int64, err error) {
	if failed, err = s.receipts.FailPendingBefore(ctx, now.Add(-pendingGiveUp), giveUpMessage); err != nil {
		return 0, 0, err
	}
	for {
		var ids []string
		err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
			if ids, err = s.receipts.TouchStalePending(ctx, pendingRetryAfter, now.Add(-pendingGiveUp), recoveryBatch); err != nil {
				return err
			}
			for _, id := range ids {
				if err := s.pub.Publish(ctx, domain.TopicReceiptUploaded, id, domain.ReceiptUploadedEvent{ReceiptID: id}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return requeued, failed, err
		}
		requeued += len(ids)
		if len(ids) < recoveryBatch {
			return requeued, failed, nil
		}
	}
}
