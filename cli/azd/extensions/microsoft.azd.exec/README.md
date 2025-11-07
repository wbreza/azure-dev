# Azure Developer CLI Exec Extension

Execute arbitrary scripts and commands with Azure Developer CLI (azd) environment variables automatically loaded into the subprocess environment.

## Overview

The `azd exec` extension allows you to run any command or script with your current azd environment variables seamlessly integrated. This is particularly useful when you need to:

- Run scripts that require Azure credentials or configuration
- Execute commands with environment-specific values
- Integrate azd environments into custom workflows
- Run language-specific tools with azd context

## Installation

```bash
azd extension install microsoft.azd.exec
```

## Usage

### Basic Syntax

```bash
azd exec <command> [args...]
```

### Examples

**Run a Python script:**
```bash
azd exec python3 main.py
```

**Run a shell script:**
```bash
# POSIX
azd exec ./deploy.sh

# Windows
azd exec .\deploy.ps1
```

**Run npm commands:**
```bash
azd exec npm run build
```

**Access environment variables in scripts:**
```bash
# POSIX
azd exec bash -c 'echo $AZURE_SUBSCRIPTION_ID'

# Windows PowerShell
azd exec pwsh -Command "Write-Host $env:AZURE_SUBSCRIPTION_ID"
```

**Run any command with azd environment:**
```bash
azd exec node server.js
azd exec go run main.go
azd exec dotnet run
```

## How It Works

1. **Environment Detection**: The extension checks for the current azd environment
2. **Variable Loading**: Retrieves all environment variables from the current azd environment
3. **Environment Merging**: Merges azd variables with the current process environment (azd values take precedence)
4. **Command Execution**: Executes the specified command as a subprocess with the merged environment
5. **Exit Code Propagation**: Returns the same exit code as the subprocess

## Environment Variables

The extension loads all azd environment variables, which typically include:

- `AZURE_SUBSCRIPTION_ID`
- `AZURE_LOCATION`
- `AZURE_RESOURCE_GROUP`
- `AZURE_TENANT_ID`
- Custom variables defined in your azd environment

## Error Handling

### No Environment Found

```
Error: no azd environment found

Run azd env new to create a new environment
```

**Solution**: Create an azd environment with `azd env new`

### No Command Specified

```
Error: no command specified

Usage: azd exec <command> [args...]
```

**Solution**: Provide a command to execute

### Command Not Found

The extension will pass through the operating system's error message if the command cannot be found.

## Cross-Platform Support

The extension works seamlessly on:

- **Windows** (PowerShell, Command Prompt, WSL)
- **Linux** (bash, sh, zsh, etc.)
- **macOS** (bash, zsh, etc.)

## Requirements

- Azure Developer CLI (azd) installed
- An initialized azd project with at least one environment

## Commands

### exec

Execute a command with azd environment variables loaded.

```bash
azd exec <command> [args...]
```

### version

Display the extension version.

```bash
azd exec version
```

## Examples by Use Case

### Deploy with Custom Scripts

```bash
azd exec ./scripts/custom-deploy.sh production
```

### Run Tests with Azure Resources

```bash
azd exec pytest tests/ --azure
```

### Build with Environment-Specific Configuration

```bash
azd exec npm run build:production
```

### Database Migrations

```bash
azd exec alembic upgrade head
```

### Infrastructure Validation

```bash
azd exec terraform plan
```

## Security Considerations

- The extension executes arbitrary commands as specified by the user
- All azd environment variables are exposed to the subprocess
- Ensure you trust the scripts and commands you execute
- Be cautious when running commands from untrusted sources

## Contributing

This extension is part of the [Azure Developer CLI](https://github.com/Azure/azure-dev) project. 
Contributions are welcome! Please see the main repository for contribution guidelines.

## License

This extension is licensed under the MIT License. See the LICENSE file in the Azure Developer CLI repository for details.
