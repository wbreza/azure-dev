package tasks

import (
	"context"

	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/ioc"
	"github.com/azure/azure-dev/cli/azd/pkg/output/ux"
)

type TaskFunc func(ctx context.Context, task Task) (ux.UxItem, error)

type taskConfig struct {
	task   Task
	taskFn TaskFunc
}

type Task interface {
	Run(ctx context.Context, taskFn TaskFunc) error
	Warn(ctx context.Context, message string)
	Skip(ctx context.Context, message string)
	Progress(ctx context.Context, message string)
}

type Runner struct {
	serviceLocator ioc.ServiceLocator
	configs        []*taskConfig
	console        input.Console
}

func NewRunner(console input.Console, serviceLocator ioc.ServiceLocator) *Runner {
	return &Runner{
		console:        console,
		serviceLocator: serviceLocator,
		configs:        []*taskConfig{},
	}
}

func (tr *Runner) Add(task Task, taskFn TaskFunc) *Runner {
	taskConfig := &taskConfig{
		task:   task,
		taskFn: taskFn,
	}
	tr.configs = append(tr.configs, taskConfig)

	return tr
}

func (tr *Runner) AddLongRunningTask(message string, taskFn TaskFunc) *Runner {
	var task *LongRunningTask
	if err := tr.serviceLocator.Resolve(&task); err != nil {
		panic(err)
	}

	task.message = message
	return tr.Add(task, taskFn)
}

func (tr *Runner) Start(ctx context.Context) error {
	var rootErr error

	for index, config := range tr.configs {
		// Print empty line between tasks
		if index > 0 && index < len(tr.configs) {
			tr.console.Message(ctx, "")
		}

		taskFn := config.taskFn
		if rootErr != nil {
			taskFn = func(ctx context.Context, task Task) (ux.UxItem, error) {
				task.Skip(ctx, "previous task failed")
				return nil, nil
			}
		}

		if err := config.task.Run(ctx, taskFn); err != nil && rootErr == nil {
			rootErr = err
		}
	}

	return rootErr
}
