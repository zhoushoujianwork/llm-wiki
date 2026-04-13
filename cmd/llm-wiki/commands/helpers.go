package commands

import (
	"os"

	"llm-wiki/internal/compiler"
	"llm-wiki/internal/conflicts"
	"llm-wiki/internal/feedback"
	"llm-wiki/internal/llm"
	"llm-wiki/internal/quality"
	"llm-wiki/internal/scheduler"
	"llm-wiki/internal/wiki"
)

// Global flag variables
var (
	cmdOutputFormat string
	cmdCache        bool
)
// Helper functions to create instances

func createLLMClient() *llm.AnthropicClient {
	client := llm.NewAnthropicClient()
	if client == nil {
		return &llm.AnthropicClient{}
	}
	return client
}

func createWikiStore(wikiDir string) *wiki.Store {
	return wiki.NewStore(wikiDir)
}

func createCompiler() *compiler.Compiler {
	return compiler.NewCompiler()
}

func createConflictDetector(wikiDir string) *conflicts.ConflictDetector {
	store := createWikiStore(wikiDir)
	client := createLLMClient()
	cacheDir := ""
	if h, err := os.UserHomeDir(); err == nil {
		cacheDir = h + "/.llm-wiki/.cache"
	}
	return conflicts.NewConflictDetector(client, store, cacheDir)
}

func createQualityEvaluator(wikiDir string) *quality.QualityEvaluator {
	if wikiDir == "" {
		wikiDir = getWikiDir()
	}
	store := createWikiStore(wikiDir)
	client := createLLMClient()
	cacheDir := ""
	if h, err := os.UserHomeDir(); err == nil {
		cacheDir = h + "/.llm-wiki/.quality_cache"
	}
	return quality.NewQualityEvaluator(client, store, cacheDir)
}

func createScheduler() *scheduler.Manager {
	return scheduler.NewManager("")
}

func createSchedulerWithDeps(wikiDir string) *scheduler.Manager {
	store := createWikiStore(wikiDir)
	client := createLLMClient()
	detector := conflicts.NewConflictDetector(client, store, "")
	evaluator := quality.NewQualityEvaluator(client, store, "")

	return scheduler.NewManagerWithAllDeps("", detector, evaluator, store)
}

func createFeedbackCollector(wikiDir string) *feedback.CollectorImpl {
	if wikiDir == "" {
		wikiDir = getWikiDir()
	}
	cacheDir := ""
	if h, err := os.UserHomeDir(); err == nil {
		cacheDir = h + "/.llm-wiki/.feedback_cache"
	}
	return feedback.NewCollector(cacheDir)
}
