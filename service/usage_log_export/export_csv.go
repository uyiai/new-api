package usage_log_export

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// utf8BOM 让 Excel 打开 UTF-8 CSV 时正确识别中文，避免乱码。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// CSVExporter 以流式方式导出使用日志为 CSV。
//
// 采用两阶段设计：先通过 NewCSVExporter 完成字段校验、文件名生成（此阶段
// 可安全地返回 HTTP 错误，因为尚未向响应写入任何字节）；随后调用 Stream
// 边查询边写出，每批 flush 一次，从而保持首字节及时下发、连接持续有数据，
// 规避网关（Nginx/Cloudflare 等）在大数据量导出时的读超时（504）。
type CSVExporter struct {
	fields   []fieldDefinition
	loc      *time.Location
	filter   model.LogExportFilter
	filename string
}

// NewCSVExporter 校验导出字段并准备好导出器；不写出任何数据。
func NewCSVExporter(input ExportInput) (*CSVExporter, error) {
	fields, err := resolveFields(input.Fields, input.Filter.IsAdmin)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("no export fields selected")
	}
	return &CSVExporter{
		fields:   fields,
		loc:      exportLocation(input.Timezone),
		filter:   input.Filter,
		filename: fmt.Sprintf("usage-logs-%s.csv", time.Now().Format("20060102-150405")),
	}, nil
}

// Filename 返回建议的下载文件名。
func (e *CSVExporter) Filename() string {
	return e.filename
}

// Stream 将日志以 CSV 写入 w。每写完一批数据后调用 flush（若非 nil），
// 以便将数据立即推送给客户端。返回错误时可能已写出部分数据，调用方
// 只能记录日志而无法再变更 HTTP 状态码。
func (e *CSVExporter) Stream(ctx context.Context, w io.Writer, flush func() error) error {
	// 写入 BOM 以兼容 Excel 中文显示。
	if _, err := w.Write(utf8BOM); err != nil {
		return err
	}

	cw := csv.NewWriter(w)

	header := make([]string, 0, len(e.fields))
	for _, field := range e.fields {
		header = append(header, field.Label)
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	var (
		lastCreatedAt int64
		lastID        int
		written       int
		record        = make([]string, len(e.fields))
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		logs, err := model.GetLogsForExportBatch(ctx, e.filter, lastCreatedAt, lastID, exportBatchSize, written)
		if err != nil {
			return err
		}
		if len(logs) == 0 {
			break
		}

		for _, log := range logs {
			row, err := exportRowValues(log, e.fields, e.loc)
			if err != nil {
				return err
			}
			for i, v := range row {
				record[i] = csvCellString(v)
			}
			if err := cw.Write(record); err != nil {
				return err
			}
			written++
		}

		// 每批落盘 + flush，持续向客户端/网关下发字节。
		cw.Flush()
		if err := cw.Error(); err != nil {
			return err
		}
		if flush != nil {
			if err := flush(); err != nil {
				return err
			}
		}

		lastLog := logs[len(logs)-1]
		lastCreatedAt = lastLog.CreatedAt
		lastID = lastLog.Id
		if len(logs) < exportBatchSize {
			break
		}
	}

	cw.Flush()
	return cw.Error()
}

// csvCellString 将单元格值转换为字符串，并对字符串类型做 CSV 公式注入防护。
func csvCellString(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return guardCSVFormula(val)
	default:
		// 数字等由业务生成、非用户可控，直接格式化即可。
		return stringValue(val)
	}
}

// guardCSVFormula 防止 CSV 公式注入：以 = + - @ 或制表/回车开头的字符串，
// 在电子表格软件中会被解释为公式。此处加前导单引号使其按文本处理。
func guardCSVFormula(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}
