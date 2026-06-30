// SPDX-FileCopyrightText: 2026 Alby Hernández <hola@achetronic.com>
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"hashrouter/internal/cmd/run"
	"hashrouter/internal/cmd/version"
)

const (
	descriptionShort = `A proxy to...`
	descriptionLong  = `
	A proxy to...
	`
)

func NewRootCommand(name string) *cobra.Command {
	c := &cobra.Command{
		Use:   name,
		Short: descriptionShort,
		Long:  strings.ReplaceAll(descriptionLong, "\t", ""),
	}

	c.AddCommand(
		version.NewCommand(),
		run.NewCommand(),
	)

	return c
}
