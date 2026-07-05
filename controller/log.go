package controller

import (
	"compress/gzip"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	usagelogexport "github.com/QuantumNous/new-api/service/usage_log_export"

	"github.com/gin-gonic/gin"
)

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId, upstreamRequestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, requestId, upstreamRequestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetLogExportFields(c *gin.Context) {
	common.ApiSuccess(c, usagelogexport.FieldGroups(true))
}

func GetUserLogExportFields(c *gin.Context) {
	common.ApiSuccess(c, usagelogexport.FieldGroups(false))
}

func ExportAllLogs(c *gin.Context) {
	exportLogs(c, true)
}

func ExportUserLogs(c *gin.Context) {
	exportLogs(c, false)
}

// logExportInFlight 按用户做单飞守卫：同一用户同一时刻只允许一个日志导出，
// 防止大数据量导出被并发放大压垮数据库/服务。此为单进程内存态，多实例部署
// 下不跨实例共享，作为轻量保护足够。
var (
	logExportInFlight   = make(map[int]struct{})
	logExportInFlightMu sync.Mutex
)

func acquireLogExportSlot(userId int) bool {
	logExportInFlightMu.Lock()
	defer logExportInFlightMu.Unlock()
	if _, ok := logExportInFlight[userId]; ok {
		return false
	}
	logExportInFlight[userId] = struct{}{}
	return true
}

func releaseLogExportSlot(userId int) {
	logExportInFlightMu.Lock()
	delete(logExportInFlight, userId)
	logExportInFlightMu.Unlock()
}

func exportLogs(c *gin.Context, isAdmin bool) {
	userId := c.GetInt("id")
	if !acquireLogExportSlot(userId) {
		common.ApiErrorMsg(c, "已有导出任务正在进行中，请等待完成后再试")
		return
	}
	defer releaseLogExportSlot(userId)

	input := usagelogexport.ExportInput{
		Filter:   buildLogExportFilter(c, isAdmin),
		Fields:   splitLogExportFields(c.Query("fields")),
		Timezone: c.Query("timezone"),
	}
	// CSV 走流式导出：边查边写边 flush，避免大数据量时网关读超时（504）。
	if strings.EqualFold(c.Query("format"), "csv") {
		exportLogsCSV(c, input)
		return
	}
	exportLogsXLSX(c, input)
}

func exportLogsXLSX(c *gin.Context, input usagelogexport.ExportInput) {
	file, filename, _, err := usagelogexport.BuildXLSX(c.Request.Context(), input)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer func() { _ = file.Close() }()
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(filename)))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Status(http.StatusOK)
	if err := file.Write(c.Writer); err != nil {
		common.SysError("failed to write usage log export: " + err.Error())
	}
}

func exportLogsCSV(c *gin.Context, input usagelogexport.ExportInput) {
	// 先构造导出器完成字段校验：此阶段尚未写出任何字节，可安全返回 JSON 错误。
	exporter, err := usagelogexport.NewCSVExporter(input)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	filename := exporter.Filename()
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(filename)))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	// 关闭 Nginx 代理缓冲，确保每次 flush 的分块能立即到达客户端与网关。
	c.Header("X-Accel-Buffering", "no")

	// 本路由已从全局 gzip 中间件排除，这里自己接管压缩，以便每批同时 flush
	// gzip.Writer 与底层 writer，保证真正的流式下发。CSV 压缩比很高（~8-10x），
	// 大幅降低传输量与耗时。
	var gz *gzip.Writer
	if strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")
		gz = gzip.NewWriter(c.Writer)
		defer func() { _ = gz.Close() }()
	}
	c.Status(http.StatusOK)

	flusher, _ := c.Writer.(http.Flusher)
	flush := func() error {
		if gz != nil {
			if err := gz.Flush(); err != nil {
				return err
			}
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}

	// 一旦开始流式写出便无法再改 HTTP 状态码，出错只能记录日志。
	if gz != nil {
		err = exporter.Stream(c.Request.Context(), gz, flush)
	} else {
		err = exporter.Stream(c.Request.Context(), c.Writer, flush)
	}
	if err != nil {
		common.SysError("failed to stream usage log csv export: " + err.Error())
	}
}

func splitLogExportFields(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			fields = append(fields, part)
		}
	}
	return fields
}

func buildLogExportFilter(c *gin.Context, isAdmin bool) model.LogExportFilter {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	channel, _ := strconv.Atoi(c.Query("channel"))
	filter := model.LogExportFilter{
		UserId:            c.GetInt("id"),
		IsAdmin:           isAdmin,
		LogType:           logType,
		StartTimestamp:    startTimestamp,
		EndTimestamp:      endTimestamp,
		ModelName:         c.Query("model_name"),
		TokenName:         c.Query("token_name"),
		Group:             c.Query("group"),
		RequestId:         c.Query("request_id"),
		UpstreamRequestId: c.Query("upstream_request_id"),
	}
	if isAdmin {
		filter.Username = c.Query("username")
		filter.Channel = channel
	}
	return filter
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
func SearchAllLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
func SearchUserLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "无效的令牌",
		})
		return
	}
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func GetLogsStat(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": stat.Quota,
			"rpm":   stat.Rpm,
			"tpm":   stat.Tpm,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	username := c.GetString("username")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum.Quota,
			"rpm":   quotaNum.Rpm,
			"tpm":   quotaNum.Tpm,
			//"token": tokenNum,
		},
	})
	return
}

func DeleteHistoryLogs(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}
	count, err := model.DeleteOldLog(c.Request.Context(), targetTimestamp, 100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
	return
}
