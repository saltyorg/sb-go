package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/saltyorg/sb-go/config"
	"github.com/saltyorg/sb-go/constants"
	"github.com/saltyorg/sb-go/spinners"
)

var (
	verbose bool
)

var configCmd = &cobra.Command{
	Use:   "validate-config",
	Short: "Validate Saltbox configuration files",
	Long:  `Validate Saltbox configuration files`,
	Run: func(cmd *cobra.Command, args []string) {

		// Set verbose mode in the config package.
		config.SetVerbose(verbose)

		// --- Accounts Configuration Validation ---
		accountsConfigFilePath := constants.SaltboxAccountsPath
		accountsFilename := filepath.Base(accountsConfigFilePath)

		accountsSuccessMessage := fmt.Sprintf("Validated %s", accountsFilename)
		accountsFailureMessage := fmt.Sprintf("Failed to validate %s", accountsFilename)

		err := spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
			TaskName:        fmt.Sprintf("Validating %s", accountsFilename),
			StopMessage:     accountsSuccessMessage,
			StopFailMessage: accountsFailureMessage,
		}, func() error {
			configFile, err := os.ReadFile(accountsConfigFilePath)
			if err != nil {
				return fmt.Errorf("error reading accounts config file (%s): %w", accountsConfigFilePath, err)
			}

			var inputMap map[string]interface{}
			if err := yaml.Unmarshal(configFile, &inputMap); err != nil {
				return fmt.Errorf("error unmarshaling accounts config file (%s): %w", accountsConfigFilePath, err)
			}

			var configData config.Config
			if err := yaml.Unmarshal(configFile, &configData); err != nil {
				return fmt.Errorf("error unmarshaling accounts config file (%s) into struct: %w", accountsConfigFilePath, err)
			}

			if err := config.ValidateConfig(&configData, inputMap); err != nil {
				return fmt.Errorf("accounts configuration validation error: %w", err)
			}
			return nil
		})

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// --- Advanced Settings Validation ---
		advSettingsFilePath := constants.SaltboxAdvancedSettingsPath
		advSettingsFilename := filepath.Base(advSettingsFilePath)

		advSuccessMessage := fmt.Sprintf("Validated %s", advSettingsFilename)
		advFailureMessage := fmt.Sprintf("Failed to validate %s", advSettingsFilename)

		err = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
			TaskName:        fmt.Sprintf("Validating %s", advSettingsFilename),
			StopMessage:     advSuccessMessage,
			StopFailMessage: advFailureMessage,
		}, func() error {
			advConfigFile, err := os.ReadFile(advSettingsFilePath)
			if err != nil {
				return fmt.Errorf("error reading advanced settings config file (%s): %w", advSettingsFilePath, err)
			}

			var advInputMap map[string]interface{}
			if err := yaml.Unmarshal(advConfigFile, &advInputMap); err != nil {
				return fmt.Errorf("error unmarshaling advanced settings config file (%s): %w", advSettingsFilePath, err)
			}

			var advConfigData config.AdvSettingsConfig
			if err := yaml.Unmarshal(advConfigFile, &advConfigData); err != nil {
				return fmt.Errorf("error unmarshaling advanced settings config file (%s) into struct: %w", advSettingsFilePath, err)
			}

			if err := config.ValidateAdvSettingsConfig(&advConfigData, advInputMap); err != nil {
				return fmt.Errorf("advanced settings configuration validation error: %w", err)
			}
			return nil
		})

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// --- Backup Configuration Validation ---
		backupConfigFilePath := constants.SaltboxBackupConfigPath
		backupFilename := filepath.Base(backupConfigFilePath)

		backupSuccessMessage := fmt.Sprintf("Validated %s", backupFilename)
		backupFailureMessage := fmt.Sprintf("Failed to validate %s", backupFilename)

		err = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
			TaskName:        fmt.Sprintf("Validating %s", backupFilename),
			StopMessage:     backupSuccessMessage,
			StopFailMessage: backupFailureMessage,
		}, func() error {
			backupConfigFile, err := os.ReadFile(backupConfigFilePath)
			if err != nil {
				return fmt.Errorf("error reading backup config file (%s): %w", backupConfigFilePath, err)
			}

			var backupInputMap map[string]interface{}
			if err := yaml.Unmarshal(backupConfigFile, &backupInputMap); err != nil {
				return fmt.Errorf("error unmarshaling backup config file (%s): %w", backupConfigFilePath, err)
			}

			var backupConfigData config.BackupConfig
			if err := yaml.Unmarshal(backupConfigFile, &backupConfigData); err != nil {
				return fmt.Errorf("error unmarshaling backup config file (%s) into struct: %w", backupConfigFilePath, err)
			}

			if err := config.ValidateBackupConfig(&backupConfigData, backupInputMap); err != nil {
				return fmt.Errorf("backup configuration validation error: %w", err)
			}
			return nil
		})

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// --- Hetzner VLAN Configuration Validation ---
		hetznerVLANConfigFilePath := constants.SaltboxHetznerVLANPath
		hetznerVLANFilename := filepath.Base(hetznerVLANConfigFilePath)

		hetznerVLANSuccessMessage := fmt.Sprintf("Validated %s", hetznerVLANFilename)
		hetznerVLANFailureMessage := fmt.Sprintf("Failed to validate %s", hetznerVLANFilename)

		err = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
			TaskName:        fmt.Sprintf("Validating %s", hetznerVLANFilename),
			StopMessage:     hetznerVLANSuccessMessage,
			StopFailMessage: hetznerVLANFailureMessage,
		}, func() error {
			hetznerVLANConfigFile, err := os.ReadFile(hetznerVLANConfigFilePath)
			if err != nil {
				return fmt.Errorf("error reading hetzner VLAN config file (%s): %w", hetznerVLANConfigFilePath, err)
			}

			var hetznerVLANInputMap map[string]interface{}
			if err := yaml.Unmarshal(hetznerVLANConfigFile, &hetznerVLANInputMap); err != nil {
				return fmt.Errorf("error unmarshaling hetzner VLAN config file (%s): %w", hetznerVLANConfigFilePath, err)
			}

			var hetznerVLANConfigData config.HetznerVLANConfig
			if err := yaml.Unmarshal(hetznerVLANConfigFile, &hetznerVLANConfigData); err != nil {
				return fmt.Errorf("error unmarshaling hetzner VLAN config file (%s) into struct: %w", hetznerVLANConfigFilePath, err)
			}

			if err := config.ValidateHetznerVLANConfig(&hetznerVLANConfigData, hetznerVLANInputMap); err != nil {
				return fmt.Errorf("hetzner VLAN configuration validation error: %w", err)
			}
			return nil
		})

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		// --- Settings Configuration Validation ---
		settingsFilePath := constants.SaltboxSettingsPath
		settingsFilename := filepath.Base(settingsFilePath)

		settingsSuccessMessage := fmt.Sprintf("Validated %s", settingsFilename)
		settingsFailureMessage := fmt.Sprintf("Failed to validate %s", settingsFilename)

		err = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
			TaskName:        fmt.Sprintf("Validating %s", settingsFilename),
			StopMessage:     settingsSuccessMessage,
			StopFailMessage: settingsFailureMessage,
		}, func() error {
			settingsConfigFile, err := os.ReadFile(settingsFilePath)
			if err != nil {
				return fmt.Errorf("error reading settings config file (%s): %w", settingsFilePath, err)
			}

			var settingsInputMap map[string]interface{}
			if err := yaml.Unmarshal(settingsConfigFile, &settingsInputMap); err != nil {
				return fmt.Errorf("error unmarshaling settings config file (%s): %w", settingsFilePath, err)
			}

			var settingsConfigData config.SettingsConfig
			if err := yaml.Unmarshal(settingsConfigFile, &settingsConfigData); err != nil {
				return fmt.Errorf("error unmarshaling settings config file (%s) into struct: %w", settingsFilePath, err)
			}

			if err := config.ValidateSettingsConfig(&settingsConfigData, settingsInputMap); err != nil {
				return fmt.Errorf("settings configuration validation error: %w", err)
			}
			return nil
		})

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	// Add the -v flag as a persistent flag to the config command.
	configCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}
