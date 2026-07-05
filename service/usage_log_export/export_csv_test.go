package usage_log_export

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// countingWriter 记录写入字节数；配套的 flush 计数用于证明分批流式下发。
type countingWriter struct{ n int }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.n += len(p)
	return len(p), nil
}

func setupExportTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	prevDB, prevLogDB := model.DB, model.LOG_DB
	prevSQLite := common.UsingSQLite
	common.UsingSQLite = true

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	t.Cleanup(func() {
		model.DB = prevDB
		model.LOG_DB = prevLogDB
		common.UsingSQLite = prevSQLite
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// TestCSVStreamFlushesPerBatch 插入超过一个批次的数据，确认 Stream 会分多次 flush，
// 即真正的边生成边下发（而非攒完再发），这是规避网关读超时的关键。
func TestCSVStreamFlushesPerBatch(t *testing.T) {
	db := setupExportTestDB(t)
	const userId = 77
	total := exportBatchSize + 50

	rows := make([]*model.Log, 0, total)
	for i := 0; i < total; i++ {
		rows = append(rows, &model.Log{
			UserId:    userId,
			Type:      model.LogTypeConsume,
			CreatedAt: int64(1700000000 + i),
			ModelName: "m",
		})
	}
	require.NoError(t, db.CreateInBatches(rows, 500).Error)

	exporter, err := NewCSVExporter(ExportInput{
		Filter: model.LogExportFilter{UserId: userId, IsAdmin: false},
		Fields: []string{"created_at", "model_name"},
	})
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(exporter.Filename(), ".csv"))

	w := &countingWriter{}
	flushCount := 0
	err = exporter.Stream(context.Background(), w, func() error {
		flushCount++
		return nil
	})
	require.NoError(t, err)

	// total = 2000 + 50 => 两个批次 => 至少 2 次 flush
	require.GreaterOrEqual(t, flushCount, 2, "streaming export must flush across multiple batches")
	require.Greater(t, w.n, 0)
}

// TestCSVStreamContextCancel 验证已取消的 context 会中止导出（不会写满整份数据）。
func TestCSVStreamContextCancel(t *testing.T) {
	db := setupExportTestDB(t)
	const userId = 78
	rows := make([]*model.Log, 0, exportBatchSize+10)
	for i := 0; i < exportBatchSize+10; i++ {
		rows = append(rows, &model.Log{UserId: userId, Type: model.LogTypeConsume, CreatedAt: int64(1700000000 + i), ModelName: "m"})
	}
	require.NoError(t, db.CreateInBatches(rows, 500).Error)

	exporter, err := NewCSVExporter(ExportInput{
		Filter: model.LogExportFilter{UserId: userId, IsAdmin: false},
		Fields: []string{"created_at"},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消
	err = exporter.Stream(ctx, &countingWriter{}, nil)
	require.ErrorIs(t, err, context.Canceled)
}

// TestNewCSVExporterRejectsBadFields 验证字段校验在写出任何字节前就返回错误。
func TestNewCSVExporterRejectsBadFields(t *testing.T) {
	_, err := NewCSVExporter(ExportInput{
		Filter: model.LogExportFilter{IsAdmin: false},
		Fields: []string{"record_id"}, // admin-only，自助导出应拒绝
	})
	require.Error(t, err)
}
