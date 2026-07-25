// Package transliter builds translation-only prompts for Hy-MT2 and validates
// the structural contract of model output.
//
// The package contains no transport, model client, CLI, or server behavior.
// Applications can use it from command-line and HTTP entry points without
// coupling prompt rules to a specific inference backend.
package transliter
