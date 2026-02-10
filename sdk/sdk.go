// Package sdk provides the embedded Python provisioning SDK.
// The SDK is pushed into containers during app installation.
package sdk

import "embed"

// PythonFS contains the Python SDK files (appstore/ package).
// The files are at python/appstore/*.py within this filesystem.
//
//go:embed python/appstore/*.py
var PythonFS embed.FS
