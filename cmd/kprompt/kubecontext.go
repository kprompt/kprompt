package main

// resolveKubeContext prefers the command --context flag, then root persistent
// --context, then the value from the config file.
func resolveKubeContext(flagCtx, rootCtx, fileCtx string) string {
	if flagCtx != "" {
		return flagCtx
	}
	if rootCtx != "" {
		return rootCtx
	}
	return fileCtx
}
