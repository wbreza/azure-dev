package prompt

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"

	"dario.cat/mergo"
	"github.com/azure/azure-dev/cli/azd/pkg/account"
	"github.com/azure/azure-dev/cli/azd/pkg/auth"
	"github.com/azure/azure-dev/cli/azd/pkg/azapi"
	"github.com/azure/azure-dev/cli/azd/pkg/ux"
	"github.com/fatih/color"
)

var (
	ErrNoResourcesFound   = fmt.Errorf("no resources found")
	ErrNoResourceSelected = fmt.Errorf("no resource selected")
)

// ResourceOptions contains options for prompting the user to select a resource.
type ResourceOptions struct {
	// ResourceType is the type of resource to select.
	ResourceType *azapi.AzureResourceType
	// Kinds is a list of resource kinds to filter by.
	Kinds []string
	// ResourceTypeDisplayName is the display name of the resource type.
	ResourceTypeDisplayName string
	// SelectorOptions contains options for the resource selector.
	SelectorOptions *SelectOptions
	// CreateResource is a function that creates a new resource.
	CreateResource func(ctx context.Context) (*azapi.ResourceExtended, error)
	// Selected is a function that determines if a resource is selected
	Selected func(resource *azapi.ResourceExtended) bool
}

// CustomResourceOptions contains options for prompting the user to select a custom resource.
type CustomResourceOptions[T any] struct {
	// SelectorOptions contains options for the resource selector.
	SelectorOptions *SelectOptions
	// LoadData is a function that loads the resource data.
	LoadData func(ctx context.Context) ([]*T, error)
	// DisplayResource is a function that displays the resource.
	DisplayResource func(resource *T) (string, error)
	// SortResource is a function that sorts the resources.
	SortResource func(a *T, b *T) int
	// Selected is a function that determines if a resource is selected
	Selected func(resource *T) bool
	// CreateResource is a function that creates a new resource.
	CreateResource func(ctx context.Context) (*T, error)
}

// ResourceGroupOptions contains options for prompting the user to select a resource group.
type ResourceGroupOptions struct {
	// SelectorOptions contains options for the resource group selector.
	SelectorOptions *SelectOptions
}

// SelectOptions contains options for prompting the user to select a resource.
type SelectOptions struct {
	// ForceNewResource specifies whether to force the user to create a new resource.
	ForceNewResource *bool
	// AllowNewResource specifies whether to allow the user to create a new resource.
	AllowNewResource *bool
	// NewResourceMessage is the message to display to the user when creating a new resource.
	NewResourceMessage string
	// CreatingMessage is the message to display to the user when creating a new resource.
	CreatingMessage string
	// Message is the message to display to the user.
	Message string
	// HelpMessage is the help message to display to the user.
	HelpMessage string
	// LoadingMessage is the loading message to display to the user.
	LoadingMessage string
	// DisplayNumbers specifies whether to display numbers next to the choices.
	DisplayNumbers *bool
	// DisplayCount is the number of choices to display at a time.
	DisplayCount int
}

type ResourceSelection[T any] struct {
	Resource *T
	Exists   bool
}

type PromptService struct {
	authManager         *auth.Manager
	subscriptionService *account.SubscriptionsService
}

func NewPromptService(authManager *auth.Manager, subscriptionService *account.SubscriptionsService) *PromptService {
	return &PromptService{
		authManager:         authManager,
		subscriptionService: subscriptionService,
	}
}

func (ps *PromptService) PromptSubscription(ctx context.Context, selectorOptions *SelectOptions) (*account.Subscription, error) {
	mergedOptions := &SelectOptions{}
	if selectorOptions == nil {
		selectorOptions = &SelectOptions{}
	}

	defaultOptions := &SelectOptions{
		Message:          "Select subscription",
		LoadingMessage:   "Loading subscriptions...",
		HelpMessage:      "Choose an Azure subscription for your project.",
		AllowNewResource: ux.Ptr(false),
	}

	mergo.Merge(mergedOptions, selectorOptions, mergo.WithoutDereference)
	mergo.Merge(mergedOptions, defaultOptions, mergo.WithoutDereference)

	var defaultSubscriptionId = ""

	return PromptCustomResource(ctx, CustomResourceOptions[account.Subscription]{
		SelectorOptions: mergedOptions,
		LoadData: func(ctx context.Context) ([]*account.Subscription, error) {
			userClaims, err := ps.authManager.ClaimsForCurrentUser(ctx, nil)
			if err != nil {
				return nil, err
			}

			subscriptionList, err := ps.subscriptionService.ListSubscriptions(ctx, userClaims.TenantId)
			if err != nil {
				return nil, err
			}

			subscriptions := make([]*account.Subscription, len(subscriptionList))
			for i, subscription := range subscriptionList {
				subscriptions[i] = &account.Subscription{
					Id:                 *subscription.SubscriptionID,
					Name:               *subscription.DisplayName,
					TenantId:           *subscription.TenantID,
					UserAccessTenantId: userClaims.TenantId,
				}
			}

			return subscriptions, nil
		},
		DisplayResource: func(subscription *account.Subscription) (string, error) {
			return fmt.Sprintf("%s %s", subscription.Name, color.HiBlackString("(%s)", subscription.Id)), nil
		},
		Selected: func(subscription *account.Subscription) bool {
			return strings.EqualFold(subscription.Id, defaultSubscriptionId)
		},
	})
}

// PromptCustomResource prompts the user to select a custom resource from a list of resources.
func PromptCustomResource[T any](ctx context.Context, options CustomResourceOptions[T]) (*T, error) {
	mergedSelectorOptions := &SelectOptions{}

	if options.SelectorOptions == nil {
		options.SelectorOptions = &SelectOptions{}
	}

	defaultSelectorOptions := &SelectOptions{
		Message:            "Select resource",
		LoadingMessage:     "Loading resources...",
		HelpMessage:        "Choose a resource for your project.",
		AllowNewResource:   ux.Ptr(true),
		ForceNewResource:   ux.Ptr(false),
		NewResourceMessage: "Create new resource",
		CreatingMessage:    "Creating new resource...",
		DisplayNumbers:     ux.Ptr(true),
		DisplayCount:       10,
	}

	mergo.Merge(mergedSelectorOptions, options.SelectorOptions, mergo.WithoutDereference)
	mergo.Merge(mergedSelectorOptions, defaultSelectorOptions, mergo.WithoutDereference)

	allowNewResource := mergedSelectorOptions.AllowNewResource != nil && *mergedSelectorOptions.AllowNewResource
	forceNewResource := mergedSelectorOptions.ForceNewResource != nil && *mergedSelectorOptions.ForceNewResource

	var resources []*T
	var selectedIndex *int

	if forceNewResource {
		allowNewResource = true
		selectedIndex = ux.Ptr(0)
	} else {
		loadingSpinner := ux.NewSpinner(&ux.SpinnerOptions{
			Text: options.SelectorOptions.LoadingMessage,
		})

		err := loadingSpinner.Run(ctx, func(ctx context.Context) error {
			resourceList, err := options.LoadData(ctx)
			if err != nil {
				return err
			}

			resources = resourceList
			return nil
		})
		if err != nil {
			return nil, err
		}

		if !allowNewResource && len(resources) == 0 {
			return nil, ErrNoResourcesFound
		}

		if options.SortResource != nil {
			slices.SortFunc(resources, options.SortResource)
		}

		var defaultIndex *int
		if options.Selected != nil {
			for i, resource := range resources {
				if options.Selected(resource) {
					defaultIndex = &i
					break
				}
			}
		}

		hasCustomDisplay := options.DisplayResource != nil

		var choices []string

		if allowNewResource {
			choices = make([]string, len(resources)+1)
			choices[0] = mergedSelectorOptions.NewResourceMessage

			if defaultIndex != nil {
				*defaultIndex++
			}
		} else {
			choices = make([]string, len(resources))
		}

		for i, resource := range resources {
			var displayValue string

			if hasCustomDisplay {
				customDisplayValue, err := options.DisplayResource(resource)
				if err != nil {
					return nil, err
				}

				displayValue = customDisplayValue
			} else {
				displayValue = fmt.Sprintf("%v", resource)
			}

			if allowNewResource {
				choices[i+1] = displayValue
			} else {
				choices[i] = displayValue
			}
		}

		resourceSelector := ux.NewSelect(&ux.SelectOptions{
			Message:        mergedSelectorOptions.Message,
			HelpMessage:    mergedSelectorOptions.HelpMessage,
			DisplayCount:   mergedSelectorOptions.DisplayCount,
			DisplayNumbers: mergedSelectorOptions.DisplayNumbers,
			Allowed:        choices,
			SelectedIndex:  defaultIndex,
		})

		userSelectedIndex, err := resourceSelector.Ask()
		if err != nil {
			return nil, err
		}

		if userSelectedIndex == nil {
			return nil, ErrNoResourceSelected
		}

		selectedIndex = userSelectedIndex
	}

	var selectedResource *T

	// Create new resource
	if allowNewResource && *selectedIndex == 0 {
		if options.CreateResource == nil {
			return nil, fmt.Errorf("no create resource function provided")
		}

		createdResource, err := options.CreateResource(ctx)
		if err != nil {
			return nil, err
		}

		selectedResource = createdResource
	} else {
		// If a new resource is allowed, decrement the selected index
		if allowNewResource {
			*selectedIndex--
		}

		selectedResource = resources[*selectedIndex]
	}

	log.Printf("Selected resource: %v", *selectedResource)

	return selectedResource, nil
}
