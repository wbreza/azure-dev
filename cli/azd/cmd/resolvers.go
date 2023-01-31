package cmd

import (
	"fmt"

	"github.com/azure/azure-dev/cli/azd/pkg/ioc"
	"github.com/spf13/cobra"
)

type FlagsResolver interface {
	Register(container ioc.NestedContainer, name string) error
	Bind(container ioc.NestedContainer, command *cobra.Command) error
}

func NewFlagsResolver[T Flags](resolver any) FlagsResolver {
	return &flagResolver[T]{
		resolver: resolver,
	}
}

type flagResolver[T Flags] struct {
	resolver any
}

func (fr *flagResolver[T]) Register(container ioc.NestedContainer, name string) error {
	err := container.RegisterNamedSingleton(name, fr.resolver)
	if err != nil {
		return err
	}

	container.RegisterSingleton(func() (T, error) {
		var zero T
		var action Flags
		err := container.ResolveNamed(name, &action)
		if err != nil {
			return zero, err
		}

		instance, ok := action.(T)
		if !ok {
			return zero, fmt.Errorf("failed converting flags to '%T'", zero)
		}

		return instance, nil
	})

	return nil
}

func (fr *flagResolver[T]) Bind(container ioc.NestedContainer, command *cobra.Command) error {
	var instance T
	err := container.Resolve(&instance)
	if err != nil {
		return err
	}

	instance.Bind(command.Flags())

	return nil
}
