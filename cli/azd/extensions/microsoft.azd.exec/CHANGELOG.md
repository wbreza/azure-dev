# Changelog

All notable changes to the azd exec extension will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2025-11-07

### Added
- Initial release of the azd exec extension
- Execute arbitrary scripts and commands with azd environment variables loaded
- Cross-platform support for Windows and POSIX environments
- Automatic environment variable merging (azd values take precedence)
- Proper subprocess exit code propagation
- Error handling for missing azd environments
