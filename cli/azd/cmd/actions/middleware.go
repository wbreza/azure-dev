package actions

import (
	"context"
)

var middlewareChain []MiddlewareFn = []MiddlewareFn{}

// Executes the next middleware in the command chain
type NextFn func(ctx context.Context) (*ActionResult, error)

// An action middleware function to execute
type MiddlewareFn func(ctx context.Context, buildOptions *ActionOptions, next NextFn) (*ActionResult, error)

// Executes the middleware chain for the specified action
func RunWithMiddleware(
	ctx context.Context,
	buildOptions *ActionOptions,
	action Action,
) (*ActionResult, error) {
	chainLength := len(middlewareChain)
	index := 0

	var nextFn NextFn

	nextFn = func(nextContext context.Context) (*ActionResult, error) {
		if index < chainLength {
			middlewareFn := middlewareChain[index]
			index++
			return middlewareFn(nextContext, buildOptions, nextFn)
		} else {
			return action.Run(ctx)
		}
	}

	result, err := nextFn(ctx)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func Use(middleware MiddlewareFn) {
	middlewareChain = append(middlewareChain, middleware)
}
