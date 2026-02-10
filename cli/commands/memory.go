package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/memory"
	"github.com/spf13/cobra"
)

// MemoryCmd 记忆管理命令
var MemoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage goclaw memory",
	Long:  `View status, index, and search memory stores.`,
}

// memoryStatusCmd 显示记忆状态
var memoryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show memory index statistics",
	Long:  `Display statistics about the memory store including total entries, sources, and types.`,
	Run:   runMemoryStatus,
}

// memoryIndexCmd 重新索引记忆文件
var memoryIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Reindex memory files",
	Long:  `Rebuild the memory index from memory files (MEMORY.md and daily notes).`,
	Run:   runMemoryIndex,
}

// memorySearchCmd 语义搜索记忆
var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search over memory",
	Long:  `Perform semantic search over stored memories using vector embeddings.`,
	Args:  cobra.ExactArgs(1),
	Run:   runMemorySearch,
}

var (
	memorySearchLimit  int
	memorySearchMinScore float64
	memorySearchJSON   bool
)

func init() {
	MemoryCmd.AddCommand(memoryStatusCmd)
	MemoryCmd.AddCommand(memoryIndexCmd)
	MemoryCmd.AddCommand(memorySearchCmd)

	memorySearchCmd.Flags().IntVarP(&memorySearchLimit, "limit", "n", 10, "Maximum number of results")
	memorySearchCmd.Flags().Float64Var(&memorySearchMinScore, "min-score", 0.7, "Minimum similarity score (0-1)")
	memorySearchCmd.Flags().BoolVar(&memorySearchJSON, "json", false, "Output in JSON format")
}

// runMemoryStatus 执行记忆状态命令
func runMemoryStatus(cmd *cobra.Command, args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(home, ".goclaw", "memory", "store.db")

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Println("Memory store not initialized.")
		fmt.Println("Run 'goclaw memory index' to create the memory store.")
		return
	}

	// Load config for API key
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create embedding provider
	var provider memory.EmbeddingProvider
	var providerErr error

	apiKey := cfg.Providers.OpenAI.APIKey
	if apiKey == "" {
		apiKey = cfg.Providers.OpenRouter.APIKey
	}

	if apiKey != "" {
		providerCfg := memory.DefaultOpenAIConfig(apiKey)
		provider, providerErr = memory.NewOpenAIProvider(providerCfg)
		if providerErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to create embedding provider: %v\n", providerErr)
			os.Exit(1)
		}
	} else {
		fmt.Println("Warning: No embedding provider configured. Some features may be limited.")
	}

	// Create store
	storeConfig := memory.DefaultStoreConfig(dbPath, provider)
	store, err := memory.NewSQLiteStore(storeConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open memory store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// Get all memories
	allMemories, err := store.List(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list memories: %v\n", err)
		os.Exit(1)
	}

	// Calculate statistics
	sourceCounts := make(map[memory.MemorySource]int)
	typeCounts := make(map[memory.MemoryType]int)
	var totalEntries int

	for _, mem := range allMemories {
		sourceCounts[mem.Source]++
		typeCounts[mem.Type]++
		totalEntries++
	}

	// Display statistics
	fmt.Println("Memory Store Status")
	fmt.Println("===================")
	fmt.Printf("\nDatabase: %s\n", dbPath)
	fmt.Printf("Total Entries: %d\n\n", totalEntries)

	fmt.Println("By Source:")
	for source, count := range sourceCounts {
		fmt.Printf("  %s: %d\n", source, count)
	}

	fmt.Println("\nBy Type:")
	for memType, count := range typeCounts {
		fmt.Printf("  %s: %d\n", memType, count)
	}

	// Show provider info
	if provider != nil {
		fmt.Printf("\nEmbedding Provider:\n")
		fmt.Printf("  Dimension: %d\n", provider.Dimension())
		fmt.Printf("  Max Batch Size: %d\n", provider.MaxBatchSize())
	}

	_ = provider // Avoid unused variable warning when provider is nil
}

// runMemoryIndex 执行记忆索引命令
func runMemoryIndex(cmd *cobra.Command, args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}

	workspaceDir := filepath.Join(home, ".goclaw", "workspace")
	memoryDir := filepath.Join(workspaceDir, "memory")
	dbPath := filepath.Join(home, ".goclaw", "memory", "store.db")

	// Load config for API key
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create embedding provider
	var provider memory.EmbeddingProvider
	var providerErr error

	apiKey := cfg.Providers.OpenAI.APIKey
	if apiKey == "" {
		apiKey = cfg.Providers.OpenRouter.APIKey
	}

	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "Error: No embedding provider API key found in config.\n")
		fmt.Fprintf(os.Stderr, "Please configure OpenAI or OpenRouter API key in ~/.goclaw/config.json\n")
		os.Exit(1)
	}

	providerCfg := memory.DefaultOpenAIConfig(apiKey)
	provider, providerErr = memory.NewOpenAIProvider(providerCfg)
	if providerErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to create embedding provider: %v\n", providerErr)
		os.Exit(1)
	}

	// Create store
	storeConfig := memory.DefaultStoreConfig(dbPath, provider)
	store, err := memory.NewSQLiteStore(storeConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open memory store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// Create memory manager
	managerConfig := memory.DefaultManagerConfig(store, provider)
	manager, err := memory.NewMemoryManager(managerConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create memory manager: %v\n", err)
		os.Exit(1)
	}
	defer manager.Close()

	fmt.Println("Indexing memory files...")
	fmt.Printf("Workspace: %s\n", workspaceDir)
	fmt.Printf("Database: %s\n\n", dbPath)

	ctx := context.Background()

	// Index MEMORY.md
	longTermPath := filepath.Join(memoryDir, "MEMORY.md")
	if _, err := os.Stat(longTermPath); err == nil {
		fmt.Printf("Indexing %s...\n", longTermPath)
		if err := indexFile(ctx, manager, longTermPath, memory.MemorySourceLongTerm, memory.MemoryTypeFact); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to index %s: %v\n", longTermPath, err)
		} else {
			fmt.Println("  OK")
		}
	} else {
		fmt.Printf("No long-term memory file found (%s)\n", longTermPath)
	}

	// Index daily notes
	dailyFiles, err := filepath.Glob(filepath.Join(memoryDir, "????-??-??.md"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to find daily notes: %v\n", err)
	} else {
		fmt.Printf("\nIndexing daily notes (%d files)...\n", len(dailyFiles))
		for _, dailyFile := range dailyFiles {
			fmt.Printf("  %s...", filepath.Base(dailyFile))
			if err := indexFile(ctx, manager, dailyFile, memory.MemorySourceDaily, memory.MemoryTypeContext); err != nil {
				fmt.Fprintf(os.Stderr, "Failed: %v\n", err)
			} else {
				fmt.Println(" OK")
			}
		}
	}

	fmt.Println("\nIndexing complete!")
}

// indexFile 索引单个文件
func indexFile(ctx context.Context, manager *memory.MemoryManager, filePath string, source memory.MemorySource, memType memory.MemoryType) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	text := string(content)
	if text == "" {
		return nil
	}

	// Split into chunks (paragraphs)
	chunks := splitIntoChunks(text, 500)

	items := make([]memory.MemoryItem, 0, len(chunks))
	for i, chunk := range chunks {
		items = append(items, memory.MemoryItem{
			Text: chunk,
			Source: source,
			Type: memType,
			Metadata: memory.MemoryMetadata{
				FilePath: filePath,
				Tags: []string{"indexed"},
			},
		})

		// Add line number hint
		if i > 0 {
			items[i-1].Metadata.LineNumber = i * 10
		}
	}

	if len(items) > 0 {
		if err := manager.AddMemoryBatch(ctx, items); err != nil {
			return fmt.Errorf("failed to add memories: %w", err)
		}
	}

	return nil
}

// splitIntoChunks 将文本分割成块
func splitIntoChunks(text string, maxChunkSize int) []string {
	// Simple paragraph-based chunking
	paragraphs := splitParagraphs(text)
	chunks := make([]string, 0)
	currentChunk := ""

	for _, para := range paragraphs {
		if len(currentChunk)+len(para) > maxChunkSize && currentChunk != "" {
			chunks = append(chunks, currentChunk)
			currentChunk = para
		} else {
			if currentChunk != "" {
				currentChunk += "\n\n"
			}
			currentChunk += para
		}
	}

	if currentChunk != "" {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

// splitParagraphs 分割段落
func splitParagraphs(text string) []string {
	// Split by double newline
	paragraphs := make([]string, 0)
	current := ""

	lines := splitLines(text)
	for _, line := range lines {
		line = trimSpace(line)
		if line == "" {
			if current != "" {
				paragraphs = append(paragraphs, current)
				current = ""
			}
		} else {
			if current != "" {
				current += " "
			}
			current += line
		}
	}

	if current != "" {
		paragraphs = append(paragraphs, current)
	}

	return paragraphs
}

// Helper functions to avoid importing strings package
func splitLines(s string) []string {
	lines := make([]string, 0)
	current := ""

	for _, ch := range s {
		if ch == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		lines = append(lines, current)
	}

	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}

// runMemorySearch 执行记忆搜索命令
func runMemorySearch(cmd *cobra.Command, args []string) {
	query := args[0]

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(home, ".goclaw", "memory", "store.db")

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Memory store not initialized. Run 'goclaw memory index' first.\n")
		os.Exit(1)
	}

	// Load config for API key
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create embedding provider
	var provider memory.EmbeddingProvider
	var providerErr error

	apiKey := cfg.Providers.OpenAI.APIKey
	if apiKey == "" {
		apiKey = cfg.Providers.OpenRouter.APIKey
	}

	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "Error: No embedding provider API key found in config.\n")
		os.Exit(1)
	}

	providerCfg := memory.DefaultOpenAIConfig(apiKey)
	provider, providerErr = memory.NewOpenAIProvider(providerCfg)
	if providerErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to create embedding provider: %v\n", providerErr)
		os.Exit(1)
	}

	// Create store
	storeConfig := memory.DefaultStoreConfig(dbPath, provider)
	store, err := memory.NewSQLiteStore(storeConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open memory store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// Create memory manager
	managerConfig := memory.DefaultManagerConfig(store, provider)
	manager, err := memory.NewMemoryManager(managerConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create memory manager: %v\n", err)
		os.Exit(1)
	}
	defer manager.Close()

	// Perform search
	ctx := context.Background()
	opts := memory.DefaultSearchOptions()
	opts.Limit = memorySearchLimit
	opts.MinScore = memorySearchMinScore

	results, err := manager.Search(ctx, query, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Search failed: %v\n", err)
		os.Exit(1)
	}

	if memorySearchJSON {
		outputSearchResultsJSON(results)
		return
	}

	outputSearchResults(query, results)
}

// outputSearchResultsJSON 输出搜索结果为 JSON
func outputSearchResultsJSON(results []*memory.SearchResult) {
	data := struct {
		Query   string                 `json:"query"`
		Count   int                    `json:"count"`
		Results []*memory.SearchResult `json:"results"`
	}{
		Count:   len(results),
		Results: results,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(jsonData))
}

// outputSearchResults 输出搜索结果
func outputSearchResults(query string, results []*memory.SearchResult) {
	if len(results) == 0 {
		fmt.Printf("No results found for: %s\n", query)
		return
	}

	fmt.Printf("Search Results for: %s\n", query)
	fmt.Printf("Found %d result(s)\n\n", len(results))

	for i, result := range results {
		fmt.Printf("[%d] Score: %.2f\n", i+1, result.Score)
		fmt.Printf("    Source: %s\n", result.Source)
		fmt.Printf("    Type: %s\n", result.Type)

		if result.Metadata.FilePath != "" {
			fmt.Printf("    File: %s", result.Metadata.FilePath)
			if result.Metadata.LineNumber > 0 {
				fmt.Printf(":%d", result.Metadata.LineNumber)
			}
			fmt.Println()
		}

		if !result.CreatedAt.IsZero() {
			fmt.Printf("    Created: %s\n", result.CreatedAt.Format(time.RFC3339))
		}

		// Truncate text for display
		text := result.Text
		maxLen := 200
		if len(text) > maxLen {
			text = text[:maxLen] + "..."
		}
		fmt.Printf("    Text: %s\n\n", text)
	}
}
