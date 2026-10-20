package store

import (
	"context"
	"time"
)

// CancelPendingExpiryForDomain cancels pending expiry alerts for one domain.
func (s *Store) CancelPendingExpiryForDomain(ctx context.Context, domainID string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'canceled', updated_at = ?
		WHERE domain_id = ? AND kind = 'expiry' AND state IN ('pending', 'sending')`,
		formatTime(at), domainID)
	return err
}

// CancelPendingNotificationsForRule cancels pending alerts produced by a rule.
func (s *Store) CancelPendingNotificationsForRule(ctx context.Context, ruleID string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'canceled', updated_at = ?
		WHERE rule_id = ? AND state IN ('pending', 'sending')`, formatTime(at), ruleID)
	return err
}

// CancelPendingGlobalNotificationsForDomain cancels pending alerts for a
// domain that came from global rules.
func (s *Store) CancelPendingGlobalNotificationsForDomain(ctx context.Context, domainID string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'canceled', updated_at = ?
		WHERE domain_id = ? AND kind = 'expiry' AND state IN ('pending', 'sending')
			AND rule_id IN (SELECT id FROM alert_rules WHERE scope = 'global')`,
		formatTime(at), domainID)
	return err
}
