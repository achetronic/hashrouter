// SPDX-FileCopyrightText: 2026 Alby Hernández <hola@achetronic.com>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"

	"hashrouter/internal/cmd"

	// Enable profiling capabilities when needed.
	// This will expose profiler only in private metrics server
	_ "net/http/pprof"
)

func main() {
	baseName := filepath.Base(os.Args[0])

	err := cmd.NewRootCommand(baseName).Execute()
	cmd.CheckError(err)
}
