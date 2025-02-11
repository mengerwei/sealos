// Copyright © 2021 sealos.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"os"

	"github.com/fanux/sealos/pkg/logger"

	v1 "github.com/fanux/sealos/pkg/types/v1alpha1"
	"github.com/fanux/sealos/pkg/utils"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	Info    bool
	feature bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sealos",
	Short: "simplest way install kubernetes tools.",
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.sealos/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&feature, "feature", false, "need use new feature. ex app feature")
	rootCmd.PersistentFlags().BoolVar(&Info, "info", false, "logger ture for Info, false for Debug")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	// Find home directory.
	configPath := v1.DefaultConfigPath
	logFile := fmt.Sprintf("%s/sealos.log", configPath)
	if !utils.FileExist(configPath) {
		err := os.MkdirAll(configPath, os.ModePerm)
		if err != nil {
			fmt.Println("create default sealos config dir failed, please create it by your self mkdir -p /root/.sealos && touch /root/.sealos/config.yaml")
		}
	}
	logger.Cfg(!Info, logFile)
}
