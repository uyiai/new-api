package controller

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupLogExportTestDB 用内存 SQLite 搭建一个只含 logs 表的测试库，并在结束后还原全局状态。
func setupLogExportTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	prevDB, prevLogDB := model.DB, model.LOG_DB
	prevSQLite, prevMySQL, prevPG := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	prevRedis := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

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
		common.UsingMySQL = prevMySQL
		common.UsingPostgreSQL = prevPG
		common.RedisEnabled = prevRedis
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedExportLogs(t *testing.T, db *gorm.DB, userId int) {
	t.Helper()
	rows := []*model.Log{
		{UserId: userId, Type: model.LogTypeConsume, CreatedAt: 1700000100, ModelName: "gpt-4o", TokenName: "tok-a", Content: "普通内容", PromptTokens: 10, CompletionTokens: 5, Quota: 100},
		{UserId: userId, Type: model.LogTypeConsume, CreatedAt: 1700000200, ModelName: "claude-3", TokenName: "tok-b", Content: "=SUM(A1:A9)", PromptTokens: 1, CompletionTokens: 2, Quota: 3},
		{UserId: userId + 1, Type: model.LogTypeConsume, CreatedAt: 1700000300, ModelName: "other-user-model", Content: "leak?"},
	}
	for _, r := range rows {
		require.NoError(t, db.Create(r).Error)
	}
}

func newExportContext(rec *httptest.ResponseRecorder, target string, userId int, acceptGzip bool) *gin.Context {
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", target, nil)
	if acceptGzip {
		c.Request.Header.Set("Accept-Encoding", "gzip")
	}
	c.Set("id", userId)
	return c
}

// TestExportUserLogsCSVStreamsGzip 验证：CSV 走 gzip 流式、带 BOM、字段标签正确、
// 用户数据隔离、以及 CSV 公式注入防护。
func TestExportUserLogsCSVStreamsGzip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupLogExportTestDB(t)
	const userId = 42
	seedExportLogs(t, db, userId)

	rec := httptest.NewRecorder()
	c := newExportContext(rec, "/api/log/self/export?format=csv&fields=created_at,model_name,details_summary", userId, true)
	exportLogs(c, false)

	require.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	require.Contains(t, rec.Header().Get("Content-Type"), "text/csv")
	require.Contains(t, rec.Header().Get("Content-Disposition"), ".csv")

	gr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	require.NoError(t, err)
	decoded, err := io.ReadAll(gr)
	require.NoError(t, err)

	require.True(t, bytes.HasPrefix(decoded, []byte{0xEF, 0xBB, 0xBF}), "should start with UTF-8 BOM")
	text := string(decoded)
	// 字段标签
	require.Contains(t, text, "Time")
	require.Contains(t, text, "Model")
	require.Contains(t, text, "Details")
	// 数据行
	require.Contains(t, text, "gpt-4o")
	require.Contains(t, text, "claude-3")
	require.Contains(t, text, "普通内容")
	// 公式注入防护：以 = 开头的内容被加前导单引号
	require.Contains(t, text, "'=SUM(A1:A9)")
	// 用户隔离：其他用户的数据不应出现
	require.NotContains(t, text, "other-user-model")
}

// TestExportUserLogsCSVPlainWhenNoGzip 验证：客户端不支持 gzip 时退回纯 CSV 流。
func TestExportUserLogsCSVPlainWhenNoGzip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupLogExportTestDB(t)
	const userId = 43
	seedExportLogs(t, db, userId)

	rec := httptest.NewRecorder()
	c := newExportContext(rec, "/api/log/self/export?format=csv&fields=created_at,model_name", userId, false)
	exportLogs(c, false)

	require.Empty(t, rec.Header().Get("Content-Encoding"))
	require.Contains(t, rec.Header().Get("Content-Type"), "text/csv")
	body := rec.Body.Bytes()
	require.True(t, bytes.HasPrefix(body, []byte{0xEF, 0xBB, 0xBF}), "plain CSV should still carry BOM")
	require.Contains(t, string(body), "gpt-4o")
}

// TestExportLogsXLSXRowCap 验证：XLSX 超过行数上限时拒绝并返回可读的引导错误。
func TestExportLogsXLSXRowCap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupLogExportTestDB(t)
	const userId = 7
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&model.Log{
			UserId: userId, Type: model.LogTypeConsume, CreatedAt: int64(1700000000 + i), ModelName: "m",
		}).Error)
	}

	prevCap := common.DataExportMaxRows
	common.DataExportMaxRows = 1
	t.Cleanup(func() { common.DataExportMaxRows = prevCap })

	rec := httptest.NewRecorder()
	// 不带 format => 默认 XLSX 路径
	c := newExportContext(rec, "/api/log/self/export?fields=created_at", userId, true)
	exportLogs(c, false)

	require.Equal(t, 200, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "\"success\":false")
	require.Contains(t, body, "超过 XLSX 导出上限")
}

// TestExportLogsCSVIgnoresRowCap 验证：CSV 流式导出不受 XLSX 行数上限约束。
func TestExportLogsCSVIgnoresRowCap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupLogExportTestDB(t)
	const userId = 8
	seedExportLogs(t, db, userId)

	prevCap := common.DataExportMaxRows
	common.DataExportMaxRows = 1
	t.Cleanup(func() { common.DataExportMaxRows = prevCap })

	rec := httptest.NewRecorder()
	c := newExportContext(rec, "/api/log/self/export?format=csv&fields=created_at,model_name", userId, false)
	exportLogs(c, false)

	require.Contains(t, rec.Header().Get("Content-Type"), "text/csv")
	require.Contains(t, rec.Body.String(), "gpt-4o")
}

// TestLogExportSingleFlight 验证单用户导出并发守卫的获取/释放语义。
func TestLogExportSingleFlight(t *testing.T) {
	require.True(t, acquireLogExportSlot(1001))
	require.False(t, acquireLogExportSlot(1001), "same user second acquire must be rejected")
	releaseLogExportSlot(1001)
	require.True(t, acquireLogExportSlot(1001), "after release should acquire again")
	releaseLogExportSlot(1001)

	// 不同用户互不影响
	require.True(t, acquireLogExportSlot(1002))
	require.True(t, acquireLogExportSlot(1003))
	releaseLogExportSlot(1002)
	releaseLogExportSlot(1003)
}
