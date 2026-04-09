package commands

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var cfgFile string

// Config holds all llm-wiki settings.
type Config struct {
	AnthropicBaseURL string `yaml:"anthropic_base_url"`
	AnthropicAPIKey  string `yaml:"anthropic_api_key"`
	AnthropicModel   string `yaml:"anthropic_model"`
	WikiDir          string `yaml:"wiki_dir"`
	SourcesDir       string `yaml:"sources_dir"`
}

// autoLoadConfig loads config once and caches it.
var cachedConfig *Config

func autoLoadConfig() *Config {
	if cachedConfig != nil {
		return cachedConfig
	}
	homeDir, _ := os.UserHomeDir()
	defaultConfigPaths := []string{
		"llm-wiki.yaml",
		filepath.Join(homeDir, ".llm-wiki", "llm-wiki.yaml"),
		filepath.Join(homeDir, ".openclaw", "workspace", "skills", "llm-wiki", "llm-wiki.yaml"),
	}
	paths := defaultConfigPaths
	if cfgFile != "" {
		paths = append([]string{cfgFile}, paths...)
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		// Apply to env so LLM client picks it up
		if cfg.AnthropicBaseURL != "" {
			os.Setenv("ANTHROPIC_BASE_URL", cfg.AnthropicBaseURL)
		}
		if cfg.AnthropicAPIKey != "" {
			os.Setenv("ANTHROPIC_API_KEY", cfg.AnthropicAPIKey)
		}
		if cfg.AnthropicModel != "" {
			os.Setenv("ANTHROPIC_MODEL", cfg.AnthropicModel)
		}
		cachedConfig = &cfg
		return &cfg
	}
	return nil
}

// loadConfig reads the config file and sets environment variables.
func loadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}

	if cfg.AnthropicBaseURL != "" {
		os.Setenv("ANTHROPIC_BASE_URL", cfg.AnthropicBaseURL)
	}
	if cfg.AnthropicAPIKey != "" {
		os.Setenv("ANTHROPIC_API_KEY", cfg.AnthropicAPIKey)
	}
	if cfg.AnthropicModel != "" {
		os.Setenv("ANTHROPIC_MODEL", cfg.AnthropicModel)
	}
	if cfg.WikiDir != "" {
		os.Setenv("LLM_WIKI_DIR", cfg.WikiDir)
	}
	if cfg.SourcesDir != "" {
		os.Setenv("LLM_WIKI_SOURCES_DIR", cfg.SourcesDir)
	}

	return nil
}

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "llm-wiki",
		Short: "LLM Wiki — Build a personal Wikipedia with LLMs",
		Long:  `Feed documents in, get a searchable wiki out. Supports GitHub repos, local files, and URLs.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
				homeDir, _ := os.UserHomeDir()
				configPaths := []string{
					cfgFile,
					"llm-wiki.yaml",
					filepath.Join(homeDir, ".llm-wiki", "llm-wiki.yaml"),
					filepath.Join(homeDir, ".openclaw", "workspace", "skills", "llm-wiki", "llm-wiki.yaml"),
				}
			for _, p := range configPaths {
				if p == "" {
					continue
				}
				if err := loadConfig(p); err == nil {
					return nil
				}
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&cfgFile, "config", "", "config file")

	root.AddCommand(NewSourceCmd())
	root.AddCommand(NewCompileCmd())
	root.AddCommand(NewQueryCmd())
	root.AddCommand(NewAskCmd())
	root.AddCommand(NewVersionCmd())

	return root
}

func getWikiDir() string {
	if dir := os.Getenv("LLM_WIKI_DIR"); dir != "" {
		return dir
	}
	if cfg := autoLoadConfig(); cfg != nil && cfg.WikiDir != "" {
		return cfg.WikiDir
	}
	return "."
}

func getSourcesDir() string {
	if dir := os.Getenv("LLM_WIKI_SOURCES_DIR"); dir != "" {
		return dir
	}
	if cfg := autoLoadConfig(); cfg != nil && cfg.SourcesDir != "" {
		return cfg.SourcesDir
	}
	return "./sources"
}
