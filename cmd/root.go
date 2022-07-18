/*
Copyright © 2021 Mark Ignacio <mark@ignacio.io>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"os"
	"strings"

	"github.com/mark-ignacio/tailscale-cloudflare/sync"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tailscale2cloudflare",
	Short: "Synchronizes Tailscale device lists with a Cloudflare (sub)domain.",
	Long: `Specify command line flags or env vars in order for tailscale2cloudflare to:
1.  GET  https://api.tailscale.com/api/v2/tailnet/:tailnet/devices?fields=default
2.  For each authorized host, upsert a ${machineName}.${cloudflare-subdomain} with either
2a. POST https://api.cloudflare/com/client/v4/zones/:zone_identifier/dns_records
2b. PUT  https://api.cloudflare/com/client/v4/zones/:zone_identifier/dns_records/:identifier

See docs and flags for details.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		}
		if viper.GetBool("verbose") {
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		} else {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		}
		zerolog.LevelFieldName = viper.GetString("level-name")
	},
	Run: func(cmd *cobra.Command, args []string) {
		var (
			tsKey     = mustLoadViperString("tailscale-key", "Tailscale API key")
			tsTailnet = mustLoadViperString("tailscale-tailnet", "Tailscale tailnet")
			cfToken   = mustLoadViperString("cloudflare-token", "Cloudflare API token")
			cfZone    = mustLoadViperString("cloudflare-zone", "Cloudflare zone ID")
			cfSub     = viper.GetString("cloudflare-subdomain")
		)
		if strings.HasSuffix(cfSub, ".") || strings.HasPrefix(cfSub, ".") {
			log.Fatal().Str("cloudflare-subdomain", cfSub).Msg("Remove '.' at the start/end of this field")
		}
		err := sync.Tailscale2Cloudflare(tsKey, tsTailnet, cfToken, cfZone, cfSub, &sync.Tailscale2CloudflareOptions{
			DryRun:       viper.GetBool("dry-run"),
			UseHostnames: viper.GetBool("sync-hostnames"),
		})
		if err != nil {
			log.Fatal().Err(err).Msg("error synchronizing Tailscale -> Cloudflare records")
		}
	},
}

func mustLoadViperString(name string, humanName string) string {
	value := viper.GetString(name)
	if value == "" {
		log.Fatal().Str("viperName", name).Msgf("Must specify a %s via environment variable or flag", humanName)
	}
	return value
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)

	persistent := rootCmd.PersistentFlags()
	persistent.String("tailscale-key", "", "Tailscale API key, usually looks like `tskey-deafbeef`")
	persistent.String("tailscale-tailnet", "", "Tailscale Tailnet name")
	persistent.String("cloudflare-token", "", "Cloudflare API token")
	persistent.String("cloudflare-zone", "", "Cloudflare zone ID")
	persistent.String("cloudflare-subdomain", "", "Cloudflare subdomain. Blank means that this will update the apex.")
	// you *can* specify these as env vars but they're meant to be flags.
	persistent.BoolP("dry-run", "n", false, "perform a dry run instead of updating")
	persistent.BoolP("verbose", "v", false, "enable debug-level logging")
	persistent.String("level-name", "level", "field name for structured log message level")
	persistent.Bool("sync-hostnames", false, "retain old behavior of syncing hostnames instead of unique machine names")
	viper.BindPFlags(persistent)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
