package azdgrpc

import (
	"context"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	gengrpc "github.com/azure/azure-dev/cli/azd/pkg/azdext/gen/grpc"
)

type promptService struct {
	gengrpc.UnimplementedPromptServiceServer
	prompter *azdext.PromptService
}

func NewPromptService(prompter *azdext.PromptService) gengrpc.PromptServiceServer {
	return &promptService{
		prompter: prompter,
	}
}

func (s *promptService) PromptSubscription(ctx context.Context, req *gengrpc.PromptSubscriptionRequest) (*gengrpc.SubscriptionResponse, error) {
	selectedSubscription, err := s.prompter.PromptSubscription(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &gengrpc.SubscriptionResponse{
		Subscription: &gengrpc.Subscription{
			Id:                 selectedSubscription.Id,
			Name:               selectedSubscription.Name,
			TenantId:           selectedSubscription.TenantId,
			UserAccessTenantId: selectedSubscription.UserAccessTenantId,
			IsDefault:          selectedSubscription.IsDefault,
		},
	}, nil
}
