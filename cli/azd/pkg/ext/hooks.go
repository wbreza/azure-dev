package ext

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/exec"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/osutil"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/azure/azure-dev/cli/azd/pkg/tools"
	"github.com/azure/azure-dev/cli/azd/pkg/tools/bash"
	"github.com/azure/azure-dev/cli/azd/pkg/tools/powershell"
	"golang.org/x/exp/slices"
)

// Generic action function that may return an error
type InvokeFn func() error

// The type of hooks. Supported values are 'pre' and 'post'
type HookType string

const (
	// Executes pre hooks
	HookTypePre HookType = "pre"
	// Execute post hooks
	HookTypePost HookType = "post"
)

var (
	ErrScriptTypeUnknown     error = errors.New("unable to determine script type. Ensure 'Type' parameter is set in configuration options")
	ErrScriptRequired        error = errors.New("script is required when location is set to 'Inline'")
	ErrPathRequired          error = errors.New("path is required when location is set to 'Path'")
	ErrUnsupportedScriptType error = errors.New("script type is not valid. Only '.sh' and '.ps1' are supported")
)

// Hooks enable support to invoke integration scripts before & after commands
// Scripts can be invoked at the project or service level or
type Hooks struct {
	commandRunner exec.CommandRunner
	console       input.Console
	cwd           string
	scripts       map[string]*ScriptConfig
	envVars       []string
}

// NewHooks creates a new instance of CommandHooks
// When `cwd` is empty defaults to current shell working directory
func NewHooks(
	commandRunner exec.CommandRunner,
	console input.Console,
	scripts map[string]*ScriptConfig,
	cwd string,
	envVars []string,
) *Hooks {
	if cwd == "" {
		osWd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		cwd = osWd
	}

	return &Hooks{
		commandRunner: commandRunner,
		console:       console,
		cwd:           cwd,
		scripts:       scripts,
		envVars:       envVars,
	}
}

// Invokes an action run runs any registered pre or post script hooks for the specified command.
func (h *Hooks) Invoke(ctx context.Context, commands []string, actionFn InvokeFn) error {
	err := h.RunScripts(ctx, HookTypePre, commands)
	if err != nil {
		return fmt.Errorf("failed running pre hooks: %w", err)
	}

	err = actionFn()
	if err != nil {
		return err
	}

	err = h.RunScripts(ctx, HookTypePost, commands)
	if err != nil {
		return fmt.Errorf("failed running post hooks: %w", err)
	}

	return nil
}

// Invokes any registered script hooks for the specified hook type and command.
func (h *Hooks) RunScripts(ctx context.Context, hookType HookType, commands []string) error {
	scripts := h.getScriptsForHook(hookType, commands)
	for _, scriptConfig := range scripts {
		err := h.execScript(ctx, scriptConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

// Gets the script to execute based on the script configuration values
// For inline scripts this will also create a temporary script file to execute
func (h *Hooks) GetScript(scriptConfig *ScriptConfig) (tools.Script, error) {
	err := h.validateScriptConfig(scriptConfig)
	if err != nil {
		return nil, err
	}

	if scriptConfig.Location == ScriptLocationUnknown {
		if scriptConfig.Path != "" {
			scriptConfig.Location = ScriptLocationPath
		} else if scriptConfig.Script != "" {
			scriptConfig.Location = ScriptLocationInline
		}
	}

	if scriptConfig.Location == ScriptLocationInline {
		tempScript, err := createTempScript(scriptConfig)
		if err != nil {
			return nil, err
		}

		scriptConfig.Path = tempScript
	}

	if scriptConfig.Type == ScriptTypeUnknown {
		scriptType, err := inferScriptTypeFromFilePath(scriptConfig.Path)
		if err != nil {
			return nil, err
		}

		scriptConfig.Type = scriptType
	}

	_, err = os.Stat(scriptConfig.Path)
	if err != nil {
		return nil, fmt.Errorf("script at '%s' is invalid, %w", scriptConfig.Path, err)
	}

	switch scriptConfig.Type {
	case ScriptTypeBash:
		return bash.NewBashScript(h.commandRunner, h.cwd, h.envVars), nil
	case ScriptTypePowershell:
		return powershell.NewPowershellScript(h.commandRunner, h.cwd, h.envVars), nil
	default:
		return nil, fmt.Errorf(
			"script type '%s' is not a valid option. Only Bash and powershell scripts are supported",
			scriptConfig.Type,
		)
	}
}

func (h *Hooks) validateScriptConfig(scriptConfig *ScriptConfig) error {
	if scriptConfig.Type == ScriptTypeUnknown && scriptConfig.Path == "" {
		return ErrScriptTypeUnknown
	}

	if scriptConfig.Location == ScriptLocationInline && scriptConfig.Script == "" {
		return ErrScriptRequired
	}

	if scriptConfig.Location == ScriptLocationPath && scriptConfig.Path == "" {
		return ErrPathRequired
	}

	return nil
}

func (h *Hooks) getScriptsForHook(prefix HookType, commands []string) []*ScriptConfig {
	validHookNames := []string{}

	for _, commandName := range commands {
		// Convert things like `azd config list` => 'configlist`
		commandName = strings.TrimPrefix(commandName, "azd")
		commandName = strings.TrimSpace(commandName)
		commandName = strings.ReplaceAll(commandName, " ", "")

		validHookNames = append(validHookNames, strings.ToLower(string(prefix)+commandName))
	}

	allHooks := []*ScriptConfig{}
	explicitHooks := h.getExplicitHooks(prefix, validHookNames)
	implicitHooks := h.getImplicitHooks(prefix, validHookNames)

	allHooks = append(allHooks, explicitHooks...)

	// Only append implicit hooks that were not already wired up explicitly.
	for _, implicitHook := range implicitHooks {
		index := slices.IndexFunc(allHooks, func(hook *ScriptConfig) bool {
			return implicitHook.Name == hook.Name
		})

		if index >= 0 {
			log.Printf(
				"Skipping hook @ '%s'. An explicit hook for '%s' was already defined in azure.yaml.\n",
				implicitHook.Path,
				implicitHook.Name,
			)
			continue
		}

		allHooks = append(allHooks, implicitHook)
	}

	return allHooks
}

func (h *Hooks) getExplicitHooks(prefix HookType, validHookNames []string) []*ScriptConfig {
	matchingScripts := []*ScriptConfig{}

	// Find explicitly configured hooks from azure.yaml
	for scriptName, scriptConfig := range h.scripts {
		if scriptConfig == nil {
			continue
		}

		index := slices.IndexFunc(validHookNames, func(hookName string) bool {
			return hookName == scriptName
		})

		if index > -1 {
			// If the script config includes an OS specific configuration use that instead
			if runtime.GOOS == "windows" && scriptConfig.Windows != nil {
				scriptConfig = scriptConfig.Windows
			} else if (runtime.GOOS == "linux" || runtime.GOOS == "darwin") && scriptConfig.Posix != nil {
				scriptConfig = scriptConfig.Posix
			}

			scriptConfig.Name = scriptName
			scriptConfig.Path = strings.ReplaceAll(scriptConfig.Path, "/", string(os.PathSeparator))
			matchingScripts = append(matchingScripts, scriptConfig)
		}
	}

	return matchingScripts
}

func (h *Hooks) getImplicitHooks(prefix HookType, validHookNames []string) []*ScriptConfig {
	matchingScripts := []*ScriptConfig{}

	hooksDir := filepath.Join(h.cwd, ".azure", "hooks")
	files, err := os.ReadDir(hooksDir)
	if err != nil {
		// Most common error would be `ErrNotExist`.
		// Log error for other error conditions (Permissions, etc)
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("failed to read azd hooks directory: %s", err.Error())
		}

		return matchingScripts
	}

	// Find implicit / convention based hooks in the .azure/hooks directory
	// Find `predeploy.sh` or similar matching a hook prefix & valid command name
	for _, file := range files {
		fileName := file.Name()
		fileNameWithoutExt := strings.TrimSuffix(fileName, path.Ext(fileName))
		isDir := file.IsDir()

		index := slices.IndexFunc(validHookNames, func(hookName string) bool {
			return !isDir && hookName == fileNameWithoutExt
		})

		if index > -1 {
			scriptType, err := inferScriptTypeFromFilePath(fileName)
			if err != nil {
				log.Printf("Found script hook '%s', but type is not supported. %s\n", fileName, err.Error())
				continue
			}

			relativePath, err := filepath.Rel(h.cwd, filepath.Join(hooksDir, fileName))
			if err != nil {
				// This err should never happen since we are only looking for files inside the specified cwd.
				log.Printf("Found script hook '%s', but is outside of project folder. Error: %s\n", file.Name(), err.Error())
				continue
			}

			scriptConfig := ScriptConfig{
				Name:     fileNameWithoutExt,
				Path:     relativePath,
				Location: ScriptLocationPath,
				Type:     scriptType,
			}

			matchingScripts = append(matchingScripts, &scriptConfig)
		}
	}

	return matchingScripts
}

func (h *Hooks) execScript(ctx context.Context, scriptConfig *ScriptConfig) error {
	// Delete any temporary inline scripts after execution
	defer func() {
		if scriptConfig.Location == ScriptLocationInline {
			os.Remove(scriptConfig.Path)
		}
	}()

	script, err := h.GetScript(scriptConfig)
	if err != nil {
		return err
	}

	formatter := h.console.GetFormatter()
	consoleInteractive := formatter == nil || formatter.Kind() == output.NoneFormat
	scriptInteractive := consoleInteractive && scriptConfig.Interactive

	// When running in an interactive terminal broadcast a message to the dev to remind them that custom hooks are running.
	if consoleInteractive {
		h.console.Message(
			ctx,
			output.WithBold(
				fmt.Sprintf(
					"Executing %s hook => %s",
					output.WithHighLightFormat(scriptConfig.Name),
					output.WithHighLightFormat(scriptConfig.Path),
				),
			),
		)
	}

	log.Printf("Executing script '%s'\n", scriptConfig.Path)
	res, err := script.Execute(ctx, scriptConfig.Path, scriptInteractive)
	if err != nil {
		execErr := fmt.Errorf("'%s' hook failed with exit code: '%d', Path: '%s'. : %w", scriptConfig.Name, res.ExitCode, scriptConfig.Path, err)

		// If an error occurred log the failure but continue
		if scriptConfig.ContinueOnError {
			h.console.Message(ctx, output.WithBold(output.WithWarningFormat("WARNING: %s", execErr.Error())))
			h.console.Message(
				ctx,
				output.WithWarningFormat("Execution will continue since ContinueOnError has been set to true."),
			)
			log.Println(execErr.Error())
		} else {
			return execErr
		}
	}

	return nil
}

func inferScriptTypeFromFilePath(path string) (ScriptType, error) {
	fileExtension := filepath.Ext(path)
	switch fileExtension {
	case ".sh":
		return ScriptTypeBash, nil
	case ".ps1":
		return ScriptTypePowershell, nil
	default:
		return "", fmt.Errorf(
			"script with file extension '%s' is not valid. %w.",
			fileExtension,
			ErrUnsupportedScriptType,
		)
	}
}

func createTempScript(scriptConfig *ScriptConfig) (string, error) {
	var ext string
	scriptHeader := []string{}

	switch scriptConfig.Type {
	case ScriptTypeBash:
		ext = "sh"
		scriptHeader = []string{
			"#!/bin/sh",
		}
	case ScriptTypePowershell:
		ext = "ps1"
	}

	// Creates .azure/hooks directory if it doesn't already exist
	// In the future any scripts with names like "predeploy.sh" or similar would
	// automatically be invoked base on our hook naming convention
	directory := filepath.Join(".azure", "hooks")
	_, err := os.Stat(directory)
	if err != nil {
		err := os.MkdirAll(directory, osutil.PermissionDirectory)
		if err != nil {
			return "", fmt.Errorf("failed creating hooks directory, %w", err)
		}
	}

	// Write the temporary script file to .azure/hooks folder
	file, err := os.CreateTemp(directory, fmt.Sprintf("%s-*.%s", scriptConfig.Name, ext))
	if err != nil {
		return "", fmt.Errorf("failed creating hook file: %w", err)
	}

	defer file.Close()

	scriptBuilder := strings.Builder{}
	for _, line := range scriptHeader {
		scriptBuilder.WriteString(fmt.Sprintf("%s\n", line))
	}
	scriptBuilder.WriteString("\n")
	scriptBuilder.WriteString("# Auto generated file from Azure Developer CLI\n")
	scriptBuilder.WriteString(scriptConfig.Script)

	// Temp generated files are cleaned up automatically after script execution has completed.
	_, err = file.WriteString(scriptBuilder.String())
	if err != nil {
		return "", fmt.Errorf("failed writing hook file, %w", err)
	}

	return file.Name(), nil
}
