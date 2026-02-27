package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/smallnest/goclaw/config"
	"github.com/smallnest/goclaw/memory/qmd"
)

// MemorySearchManager 统一的记忆搜索接口
type MemorySearchManager interface {
	// Search 执行语义搜索
	Search(ctx context.Context, query string, opts SearchOptions) ([]*SearchResult, error)
	// Add 添加记忆（仅 builtin 支持）
	Add(ctx context.Context, text string, source MemorySource, memType MemoryType, metadata MemoryMetadata) error
	// GetStatus 获取状态
	GetStatus() map[string]interface{}
	// Close 关闭
	Close() error
}

// BuiltinSearchManager builtin 后端实现
type BuiltinSearchManager struct {
	manager *MemoryManager
	dbPath  string
}

// QMDSearchManager QMD 后端实现
type QMDSearchManager struct {
	qmdMgr      *qmd.QMDManager
	fallbackMgr MemorySearchManager // 回退到 builtin
	useFallback bool
	config      config.QMDConfig
	workspace   string
}

// NewBuiltinSearchManager 创建 builtin 搜索管理器
func NewBuiltinSearchManager(cfg config.MemoryConfig, workspace string) (MemorySearchManager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	dbPath := cfg.Builtin.DatabasePath
	if dbPath == "" {
		dbPath = filepath.Join(home, ".goclaw", "memory", "store.db")
	}

	// 确保数据库目录存在
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// 创建存储
	storeConfig := DefaultStoreConfig(dbPath, nil)
	store, err := NewSQLiteStore(storeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open memory store: %w", err)
	}

	// 创建管理器（不使用 provider，仅用于元数据）
	managerConfig := ManagerConfig{
		Store:        store,
		Provider:     nil, // QMD 模式下不需要本地 provider
		CacheMaxSize: 1000,
	}
	manager, err := NewMemoryManager(managerConfig)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to create memory manager: %w", err)
	}

	return &BuiltinSearchManager{
		manager: manager,
		dbPath:  dbPath,
	}, nil
}

// Search 执行搜索
func (m *BuiltinSearchManager) Search(ctx context.Context, query string, opts SearchOptions) ([]*SearchResult, error) {
	return m.manager.Search(ctx, query, opts)
}

// Add 添加记忆
func (m *BuiltinSearchManager) Add(ctx context.Context, text string, source MemorySource, memType MemoryType, metadata MemoryMetadata) error {
	_, err := m.manager.AddMemory(ctx, text, source, memType, metadata)
	return err
}

// GetStatus 获取状态
func (m *BuiltinSearchManager) GetStatus() map[string]interface{} {
	status := make(map[string]interface{})
	status["backend"] = "builtin"
	status["database_path"] = m.dbPath

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if stats, err := m.manager.GetStats(ctx); err == nil {
		status["total_count"] = stats.TotalCount
		status["cache_size"] = stats.CacheSize
	}

	return status
}

// Close 关闭管理器
func (m *BuiltinSearchManager) Close() error {
	return m.manager.Close()
}

// NewQMDSearchManager 创建 QMD 搜索管理器
func NewQMDSearchManager(qmdCfg config.QMDConfig, workspace string) (MemorySearchManager, error) {
	// 转换配置
	cfg := qmd.QMDConfig{
		Command:        qmdCfg.Command,
		Enabled:        qmdCfg.Enabled,
		IncludeDefault: qmdCfg.IncludeDefault,
		Paths:          make([]qmd.QMDPathConfig, len(qmdCfg.Paths)),
		Sessions: qmd.QMDSessionsConfig{
			Enabled:       qmdCfg.Sessions.Enabled,
			ExportDir:     qmdCfg.Sessions.ExportDir,
			RetentionDays: qmdCfg.Sessions.RetentionDays,
		},
		Update: qmd.QMDUpdateConfig{
			Interval:       qmdCfg.Update.Interval,
			OnBoot:         qmdCfg.Update.OnBoot,
			EmbedInterval:  qmdCfg.Update.EmbedInterval,
			CommandTimeout: qmdCfg.Update.CommandTimeout,
			UpdateTimeout:  qmdCfg.Update.UpdateTimeout,
		},
		Limits: qmd.QMDLimitsConfig{
			MaxResults:      qmdCfg.Limits.MaxResults,
			MaxSnippetChars: qmdCfg.Limits.MaxSnippetChars,
			TimeoutMs:       qmdCfg.Limits.TimeoutMs,
		},
	}

	for i, p := range qmdCfg.Paths {
		cfg.Paths[i] = qmd.QMDPathConfig{
			Name:    p.Name,
			Path:    p.Path,
			Pattern: p.Pattern,
		}
	}

	qmdMgr := qmd.NewQMDManager(cfg, workspace, "")

	// 尝试初始化
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := qmdMgr.Initialize(ctx); err != nil {
		// QMD 不可用，使用 fallback
		mgr, err := NewBuiltinSearchManager(config.MemoryConfig{
			Backend: "builtin",
			Builtin: config.BuiltinMemoryConfig{
				Enabled: true,
			},
		}, workspace)
		if err != nil {
			return nil, err
		}

		return &QMDSearchManager{
			qmdMgr:      qmdMgr,
			fallbackMgr: mgr,
			useFallback: true,
			config:      qmdCfg,
			workspace:   workspace,
		}, nil
	}

	return &QMDSearchManager{
		qmdMgr:      qmdMgr,
		fallbackMgr: nil,
		useFallback: false,
		config:      qmdCfg,
		workspace:   workspace,
	}, nil
}

// Search 执行搜索
func (m *QMDSearchManager) Search(ctx context.Context, query string, opts SearchOptions) ([]*SearchResult, error) {
	if m.useFallback && m.fallbackMgr != nil {
		return m.fallbackMgr.Search(ctx, query, opts)
	}

	// 使用 QMD 搜索
	qmdResults, err := m.qmdMgr.Query(ctx, query)
	if err != nil {
		// 切换到 fallback
		if m.fallbackMgr == nil {
			m.fallbackMgr, _ = NewBuiltinSearchManager(config.MemoryConfig{
				Backend: "builtin",
				Builtin: config.BuiltinMemoryConfig{
					Enabled: true,
				},
			}, m.workspace)
		}
		m.useFallback = true
		return m.fallbackMgr.Search(ctx, query, opts)
	}

	// 转换 QMD 结果为 SearchResult
	results := make([]*SearchResult, 0, len(qmdResults))
	for _, r := range qmdResults {
		result := &SearchResult{
			VectorEmbedding: VectorEmbedding{
				Text: r.Snippet,
				Metadata: MemoryMetadata{
					FilePath:   r.Path,
					LineNumber: r.Line,
				},
			},
			Score: r.Score,
		}
		results = append(results, result)
	}

	return results, nil
}

// Add 添加记忆（QMD 不支持）
func (m *QMDSearchManager) Add(ctx context.Context, text string, source MemorySource, memType MemoryType, metadata MemoryMetadata) error {
	if m.useFallback && m.fallbackMgr != nil {
		return m.fallbackMgr.Add(ctx, text, source, memType, metadata)
	}
	return fmt.Errorf("QMD backend does not support adding memories directly")
}

// GetStatus 获取状态
func (m *QMDSearchManager) GetStatus() map[string]interface{} {
	status := make(map[string]interface{})
	status["backend"] = "qmd"
	status["fallback_enabled"] = m.useFallback

	if !m.useFallback {
		qmdStatus := m.qmdMgr.GetStatus()
		status["available"] = qmdStatus.Available
		status["collections"] = qmdStatus.Collections
		status["last_updated"] = qmdStatus.LastUpdated
		status["last_embed"] = qmdStatus.LastEmbed
		status["indexed_files"] = qmdStatus.IndexedFiles
		status["total_documents"] = qmdStatus.TotalDocuments
		if qmdStatus.Error != "" {
			status["error"] = qmdStatus.Error
		}
	} else if m.fallbackMgr != nil {
		status["fallback_status"] = m.fallbackMgr.GetStatus()
	}

	return status
}

// Close 关闭管理器
func (m *QMDSearchManager) Close() error {
	var err1, err2 error
	if m.qmdMgr != nil {
		err1 = m.qmdMgr.Close()
	}
	if m.fallbackMgr != nil {
		err2 = m.fallbackMgr.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
}

// GetMemorySearchManager 根据配置创建搜索管理器
func GetMemorySearchManager(cfg config.MemoryConfig, workspace string) (MemorySearchManager, error) {
	switch cfg.Backend {
	case "qmd":
		if cfg.QMD.Enabled {
			return NewQMDSearchManager(cfg.QMD, workspace)
		}
		// 回退到 builtin
		return GetBuiltinSearchManager(cfg, workspace)
	case "builtin", "":
		return GetBuiltinSearchManager(cfg, workspace)
	default:
		return nil, fmt.Errorf("unknown memory backend: %s", cfg.Backend)
	}
}

// GetBuiltinSearchManager 获取 builtin 搜索管理器
func GetBuiltinSearchManager(cfg config.MemoryConfig, workspace string) (MemorySearchManager, error) {
	return NewBuiltinSearchManager(cfg, workspace)
}
