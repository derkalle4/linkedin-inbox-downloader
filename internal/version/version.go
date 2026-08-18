package version

// Version is set at build time via -ldflags or falls back to the embedded VERSION file.
var Version = "dev"
