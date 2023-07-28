// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/pkg/config"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/azure/azure-dev/cli/azd/pkg/templates"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func templateNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	templateManager, err := templates.NewTemplateManager(config.NewUserConfigManager())
	if err != nil {
		cobra.CompError(fmt.Sprintf("Error creating template manager: %w", err))
		return []string{}, cobra.ShellCompDirectiveError
	}

	templates, err := templateManager.ListTemplates(cmd.Context(), nil)
	if err != nil {
		cobra.CompError(fmt.Sprintf("Error listing templates: %s", err))
		return []string{}, cobra.ShellCompDirectiveError
	}

	templateNames := make([]string, len(templates))
	for i, v := range templates {
		templateNames[i] = v.Name
	}
	return templateNames, cobra.ShellCompDirectiveDefault
}

func templatesActions(root *actions.ActionDescriptor) *actions.ActionDescriptor {
	group := root.Add("template", &actions.ActionDescriptorOptions{
		Command: &cobra.Command{
			Short: fmt.Sprintf("Find and view template details. %s", output.WithWarningFormat("(Beta)")),
		},
		HelpOptions: actions.ActionHelpOptions{
			Description: getCmdTemplateHelpDescription,
		},
		GroupingOptions: actions.CommandGroupOptions{
			RootLevelHelp: actions.CmdGroupConfig,
		},
	})

	group.Add("list", &actions.ActionDescriptorOptions{
		Command:        newTemplateListCmd(),
		ActionResolver: newTemplatesListAction,
		FlagsResolver:  newTemplateListFlags,
		OutputFormats:  []output.Format{output.JsonFormat, output.TableFormat},
		DefaultFormat:  output.TableFormat,
	})

	group.Add("show", &actions.ActionDescriptorOptions{
		Command:        newTemplateShowCmd(),
		ActionResolver: newTemplatesShowAction,
		OutputFormats:  []output.Format{output.JsonFormat, output.NoneFormat},
		DefaultFormat:  output.NoneFormat,
	})

	return group
}

type templateListFlags struct {
	source string
}

func newTemplateListFlags(cmd *cobra.Command, global *internal.GlobalCommandOptions) *templateListFlags {
	flags := &templateListFlags{}
	flags.Bind(cmd.Flags(), global)

	return flags
}

func (f *templateListFlags) Bind(local *pflag.FlagSet, global *internal.GlobalCommandOptions) {
	local.StringVarP(&f.source, "source", "s", "", "Filter templates by source")
}

func newTemplateListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   fmt.Sprintf("Show list of sample azd templates. %s", output.WithWarningFormat("(Beta)")),
		Aliases: []string{"ls"},
	}
}

type templatesListAction struct {
	flags           *templateListFlags
	formatter       output.Formatter
	writer          io.Writer
	templateManager *templates.TemplateManager
}

func newTemplatesListAction(
	flags *templateListFlags,
	formatter output.Formatter,
	writer io.Writer,
	templateManager *templates.TemplateManager,
) actions.Action {
	return &templatesListAction{
		flags:           flags,
		formatter:       formatter,
		writer:          writer,
		templateManager: templateManager,
	}
}

func (tl *templatesListAction) Run(ctx context.Context) (*actions.ActionResult, error) {
	options := &templates.ListOptions{Source: tl.flags.source}
	listedTemplates, err := tl.templateManager.ListTemplates(ctx, options)
	if err != nil {
		return nil, err
	}

	if tl.formatter.Kind() == output.TableFormat {
		columns := []output.Column{
			{
				Heading:       "Repository Path",
				ValueTemplate: "{{.RepositoryPath}}",
			},
			{
				Heading:       "Source",
				ValueTemplate: "{{.Source}}",
			},
			{
				Heading:       "Name",
				ValueTemplate: "{{.Name}}",
			},
		}

		err = tl.formatter.Format(listedTemplates, tl.writer, output.TableFormatterOptions{
			Columns: columns,
		})
	} else {
		err = tl.formatter.Format(listedTemplates, tl.writer, nil)
	}

	return nil, err
}

type templatesShowAction struct {
	formatter       output.Formatter
	writer          io.Writer
	templateManager *templates.TemplateManager
	path            string
}

func newTemplatesShowAction(
	formatter output.Formatter,
	writer io.Writer,
	templateManager *templates.TemplateManager,
	args []string,
) actions.Action {
	return &templatesShowAction{
		formatter:       formatter,
		writer:          writer,
		templateManager: templateManager,
		path:            args[0],
	}
}

func (a *templatesShowAction) Run(ctx context.Context) (*actions.ActionResult, error) {
	matchingTemplate, err := a.templateManager.GetTemplate(ctx, a.path)

	if err != nil {
		return nil, err
	}

	if a.formatter.Kind() == output.NoneFormat {
		err = matchingTemplate.Display(a.writer)
	} else {
		err = a.formatter.Format(matchingTemplate, a.writer, nil)
	}

	return nil, err
}

func newTemplateShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <template>",
		Short: fmt.Sprintf("Show details for a given template. %s", output.WithWarningFormat("(Beta)")),
		Args:  cobra.ExactArgs(1),
	}
}

func getCmdTemplateHelpDescription(*cobra.Command) string {
	return generateCmdHelpDescription(
		fmt.Sprintf(
			"View details of your current template or browse a list of curated sample templates. %s",
			output.WithWarningFormat("(Beta)")),
		[]string{
			formatHelpNote(fmt.Sprintf("The azd CLI includes a curated list of sample templates viewable by running %s.",
				output.WithHighLightFormat("azd template list"))),
			formatHelpNote(fmt.Sprintf("To view all available sample templates, including those submitted by the azd"+
				" community visit: %s.",
				output.WithLinkFormat("https://azure.github.io/awesome-azd"))),
			formatHelpNote(fmt.Sprintf("Running %s without a template will prompt you to start with a minimal"+
				" template or select from our curated list of samples.",
				output.WithHighLightFormat("azd init"))),
		})
}
