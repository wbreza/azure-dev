package cmd

import (
	"context"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/pkg/account"
	"github.com/spf13/cobra"
)

func fooActions(root *actions.ActionDescriptor) *actions.ActionDescriptor {
	return root.Add("foo", &actions.ActionDescriptorOptions{
		Command: &cobra.Command{
			Use:   "foo",
			Short: "foo bar baz",
		},
		ActionResolver: newFooAction,
	})
}

type fooAction struct {
	accountManager account.Manager
}

func newFooAction(accountManager account.Manager) actions.Action {
	return &fooAction{
		accountManager: accountManager,
	}
}

func (a *fooAction) Run(ctx context.Context) (*actions.ActionResult, error) {
	subscriptions, err := a.accountManager.GetSubscriptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscriptions: %w", err)
	}

	for _, subscription := range subscriptions {
		fmt.Printf("Subscription: %s (Tenant: %s)\n", subscription.Name, subscription.TenantId)
	}

	return &actions.ActionResult{
		Message: &actions.ResultMessage{
			Header: "Completed foo successfully",
		},
	}, nil
}
