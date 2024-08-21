package run

import (
	"fmt"
	"hashrouter/internal/config"
	"hashrouter/internal/globals"
	"hashrouter/internal/proxy"
	"log"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

const (
	descriptionShort = `Execute router process`

	descriptionLong = `
	Run execute router process`

	//
	ConfigFlagErrorMessage       = "impossible to get flag --config: %s"
	ConfigNotParsedErrorMessage  = "impossible to parse config file: %s"
	LogLevelFlagErrorMessage     = "impossible to get flag --log-level: %s"
	DisableTraceFlagErrorMessage = "impossible to get flag --disable-trace: %s"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "run",
		DisableFlagsInUseLine: true,
		Short:                 descriptionShort,
		Long:                  descriptionLong,

		Run: RunCommand,
	}

	//
	cmd.Flags().String("log-level", "info", "Verbosity level for logs")
	cmd.Flags().Bool("disable-trace", true, "Disable showing traces in logs")
	cmd.Flags().String("config", "hashrouter.yaml", "Path to the YAML config file")

	return cmd
}

// RunCommand TODO
// Ref: https://pkg.go.dev/github.com/spf13/pflag#StringSlice
func RunCommand(cmd *cobra.Command, args []string) {

	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		log.Fatalf(ConfigFlagErrorMessage, err)
	}

	// Init the logger and store the level into the context
	logLevelFlag, err := cmd.Flags().GetString("log-level")
	if err != nil {
		log.Fatalf(LogLevelFlagErrorMessage, err)
	}

	disableTraceFlag, err := cmd.Flags().GetBool("disable-trace")
	if err != nil {
		log.Fatalf(DisableTraceFlagErrorMessage, err)
	}

	err = globals.SetLogger(logLevelFlag, disableTraceFlag)
	if err != nil {
		log.Fatal(err)
	}

	/////////////////////////////
	// EXECUTION FLOW RELATED
	/////////////////////////////

	globals.Application.Logger.Infof("starting hashrouter. Getting ready to route some targets")

	// Parse and store the config
	configContent, err := config.ReadFile(configPath)
	if err != nil {
		globals.Application.Logger.Fatalf(fmt.Sprintf(ConfigNotParsedErrorMessage, err))
	}
	globals.Application.Config = configContent

	//
	var waitGroup sync.WaitGroup
	for _, proxyConfig := range globals.Application.Config.Proxies {

		proxyObj := proxy.NewProxy(proxyConfig)

		waitGroup.Add(1)
		go proxyObj.Run(&waitGroup)

		time.Sleep(2 * time.Second) // TODO: unhardcode this
	}

	waitGroup.Wait()

}
