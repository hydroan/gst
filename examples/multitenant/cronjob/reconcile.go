package cronjob

import (
	"context"

	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/module/authz"
	"go.uber.org/zap"
)

// The reconcile_policies job runs every five minutes.
const (
	reconcilePoliciesName = "reconcile_policies"
	reconcilePoliciesSpec = "@every 5m"
)

// reconcilePolicies compares the stored authorization rules with the roles,
// menus and bindings they derive from, across every tenant, and logs how
// many disagree and the first of them: a diagnostic, since repairing is a
// decision for whoever reads it.
func reconcilePolicies(ctx context.Context) error {
	report, err := authz.ReconcilePolicies(ctx)
	if err != nil {
		return err
	}
	if report.InSync() {
		logger.App.Infoz("authorization rules in sync", zap.Int("checked", report.Checked), zap.Int("skipped", report.Skipped))
		return nil
	}
	logger.App.Warnz("authorization rules drifted", zap.Int("drifts", len(report.Drifts)), zap.Any("first", report.Drifts[0]))
	return nil
}
