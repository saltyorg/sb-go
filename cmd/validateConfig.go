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

		var accountsValidationError error
		if verbose {
			fmt.Printf("Validating %s...\n", accountsFilename)
			accountsValidationError = validateConfigFile(accountsConfigFilePath, &config.Config{}, config.ValidateConfig)
			if accountsValidationError == nil {
				fmt.Println(accountsSuccessMessage)
			} else {
				fmt.Println(accountsFailureMessage)
			}
		} else {
			accountsValidationError = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
				TaskName:        fmt.Sprintf("Validating %s", accountsFilename),
				StopMessage:     accountsSuccessMessage,
				StopFailMessage: accountsFailureMessage,
			}, func() error {
				return validateConfigFile(accountsConfigFilePath, &config.Config{}, config.ValidateConfig)
			})
		}

		if accountsValidationError != nil {
			fmt.Println(accountsValidationError)
			os.Exit(1)
		}

		// --- Advanced Settings Validation ---
		advSettingsFilePath := constants.SaltboxAdvancedSettingsPath
		advSettingsFilename := filepath.Base(advSettingsFilePath)

		advSuccessMessage := fmt.Sprintf("Validated %s", advSettingsFilename)
		advFailureMessage := fmt.Sprintf("Failed to validate %s", advSettingsFilename)

		var advSettingsValidationError error
		if verbose {
			fmt.Printf("Validating %s...\n", advSettingsFilename)
			advSettingsValidationError = validateConfigFile(advSettingsFilePath, &config.AdvSettingsConfig{}, config.ValidateAdvSettingsConfig)
			if advSettingsValidationError == nil {
				fmt.Println(advSuccessMessage)
			} else {
				fmt.Println(advFailureMessage)
			}
		} else {
			advSettingsValidationError = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
				TaskName:        fmt.Sprintf("Validating %s", advSettingsFilename),
				StopMessage:     advSuccessMessage,
				StopFailMessage: advFailureMessage,
			}, func() error {
				return validateConfigFile(advSettingsFilePath, &config.AdvSettingsConfig{}, config.ValidateAdvSettingsConfig)
			})
		}

		if advSettingsValidationError != nil {
			fmt.Println(advSettingsValidationError)
			os.Exit(1)
		}

		// --- Backup Configuration Validation ---
		backupConfigFilePath := constants.SaltboxBackupConfigPath
		backupFilename := filepath.Base(backupConfigFilePath)

		backupSuccessMessage := fmt.Sprintf("Validated %s", backupFilename)
		backupFailureMessage := fmt.Sprintf("Failed to validate %s", backupFilename)

		var backupValidationError error
		if verbose {
			fmt.Printf("Validating %s...\n", backupFilename)
			backupValidationError = validateConfigFile(backupConfigFilePath, &config.BackupConfig{}, config.ValidateBackupConfig)
			if backupValidationError == nil {
				fmt.Println(backupSuccessMessage)
			} else {
				fmt.Println(backupFailureMessage)
			}
		} else {
			backupValidationError = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
				TaskName:        fmt.Sprintf("Validating %s", backupFilename),
				StopMessage:     backupSuccessMessage,
				StopFailMessage: backupFailureMessage,
			}, func() error {
				return validateConfigFile(backupConfigFilePath, &config.BackupConfig{}, config.ValidateBackupConfig)
			})
		}

		if backupValidationError != nil {
			fmt.Println(backupValidationError)
			os.Exit(1)
		}

		// --- Hetzner VLAN Configuration Validation ---
		hetznerVLANConfigFilePath := constants.SaltboxHetznerVLANPath
		hetznerVLANFilename := filepath.Base(hetznerVLANConfigFilePath)

		hetznerVLANSuccessMessage := fmt.Sprintf("Validated %s", hetznerVLANFilename)
		hetznerVLANFailureMessage := fmt.Sprintf("Failed to validate %s", hetznerVLANFilename)

		var hetznerVLANValidationError error
		if verbose {
			fmt.Printf("Validating %s...\n", hetznerVLANFilename)
			hetznerVLANValidationError = validateConfigFile(hetznerVLANConfigFilePath, &config.HetznerVLANConfig{}, config.ValidateHetznerVLANConfig)
			if hetznerVLANValidationError == nil {
				fmt.Println(hetznerVLANSuccessMessage)
			} else {
				fmt.Println(hetznerVLANFailureMessage)
			}
		} else {
			hetznerVLANValidationError = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
				TaskName:        fmt.Sprintf("Validating %s", hetznerVLANFilename),
				StopMessage:     hetznerVLANSuccessMessage,
				StopFailMessage: hetznerVLANFailureMessage,
			}, func() error {
				return validateConfigFile(hetznerVLANConfigFilePath, &config.HetznerVLANConfig{}, config.ValidateHetznerVLANConfig)
			})
		}

		if hetznerVLANValidationError != nil {
			fmt.Println(hetznerVLANValidationError)
			os.Exit(1)
		}

		// --- Settings Configuration Validation ---
		settingsFilePath := constants.SaltboxSettingsPath
		settingsFilename := filepath.Base(settingsFilePath)

		settingsSuccessMessage := fmt.Sprintf("Validated %s", settingsFilename)
		settingsFailureMessage := fmt.Sprintf("Failed to validate %s", settingsFilename)

		var settingsValidationError error
		if verbose {
			fmt.Printf("Validating %s...\n", settingsFilename)
			settingsValidationError = validateConfigFile(settingsFilePath, &config.SettingsConfig{}, config.ValidateSettingsConfig)
			if settingsValidationError == nil {
				fmt.Println(settingsSuccessMessage)
			} else {
				fmt.Println(settingsFailureMessage)
			}
		} else {
			settingsValidationError = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
				TaskName:        fmt.Sprintf("Validating %s", settingsFilename),
				StopMessage:     settingsSuccessMessage,
				StopFailMessage: settingsFailureMessage,
			}, func() error {
				return validateConfigFile(settingsFilePath, &config.SettingsConfig{}, config.ValidateSettingsConfig)
			})
		}

		if settingsValidationError != nil {
			fmt.Println(settingsValidationError)
			os.Exit(1)
		}

		// --- MOTD Configuration Validation ---
		motdConfigPath := constants.SaltboxMOTDPath
		motdFilename := filepath.Base(motdConfigPath)

		// Skip this validation if the file doesn't exist yet
		if _, err := os.Stat(motdConfigPath); err == nil {
			motdSuccessMessage := fmt.Sprintf("Validated %s", motdFilename)
			motdFailureMessage := fmt.Sprintf("Failed to validate %s", motdFilename)

			var motdValidationError error
			if verbose {
				fmt.Printf("Validating %s...\n", motdFilename)
				motdValidationError = validateConfigFile(motdConfigPath, &config.MOTDConfig{}, config.ValidateMOTDConfig)
				if motdValidationError == nil {
					fmt.Println(motdSuccessMessage)
				} else {
					fmt.Println(motdFailureMessage)
				}
			} else {
				motdValidationError = spinners.RunTaskWithSpinnerCustom(spinners.SpinnerOptions{
					TaskName:        fmt.Sprintf("Validating %s", motdFilename),
					StopMessage:     motdSuccessMessage,
					StopFailMessage: motdFailureMessage,
				}, func() error {
					return validateConfigFile(motdConfigPath, &config.MOTDConfig{}, config.ValidateMOTDConfig)
				})
			}

			if motdValidationError != nil {
				fmt.Println(motdValidationError)
				os.Exit(1)
			}
		} else if verbose {
			fmt.Printf("MOTD config file not found at %s, skipping validation\n", motdConfigPath)
		}
	},
}

type validationFunc[T any] func(config *T, input map[string]interface{}) error

func validateConfigFile[T any](filePath string, configData *T, validateFn validationFunc[T]) error {
	configFile, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading config file (%s): %w", filePath, err)
	}

	var inputMap map[string]interface{}
	if err := yaml.Unmarshal(configFile, &inputMap); err != nil {
		return fmt.Errorf("error unmarshaling config file (%s): %w", filePath, err)
	}

	if err := yaml.Unmarshal(configFile, configData); err != nil {
		return fmt.Errorf("error unmarshaling config file (%s) into struct: %w", filePath, err)
	}

	if err := validateFn(configData, inputMap); err != nil {
		return fmt.Errorf("configuration validation error: %w", err)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(configCmd)
	// Add the -v flag as a persistent flag to the config command.
	configCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}
