package azdgrpc

import (
	"context"

	azdprompt "github.com/azure/azure-dev/cli/azd/pkg/azdext/gen/azdprompt"
	"github.com/azure/azure-dev/cli/azd/pkg/prompt"
)

type promptService struct {
	azdprompt.UnimplementedPromptServiceServer
	prompter *prompt.PromptService
}

func NewPromptService(prompter *prompt.PromptService) azdprompt.PromptServiceServer {
	return &promptService{
		prompter: prompter,
	}
}

func (s *promptService) PromptSubscription(ctx context.Context, req *azdprompt.PromptSubscriptionRequest) (*azdprompt.SubscriptionResponse, error) {
	selectedSubscription, err := s.prompter.PromptSubscription(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &azdprompt.SubscriptionResponse{
		Subscription: &azdprompt.Subscription{
			Id:                 selectedSubscription.Id,
			Name:               selectedSubscription.Name,
			TenantId:           selectedSubscription.TenantId,
			UserAccessTenantId: selectedSubscription.UserAccessTenantId,
			IsDefault:          selectedSubscription.IsDefault,
		},
	}, nil
}
