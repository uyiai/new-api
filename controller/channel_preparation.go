package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type channelPreparationCreateRequest struct {
	model.ChannelPreparation
	Groups []string `json:"groups"`
}

type channelPreparationImportRequest struct {
	Items []model.ChannelPreparation `json:"items"`
}

type channelPreparationBatchRequest struct {
	Ids []int `json:"ids"`
}

type channelPreparationImportResult struct {
	Index int                               `json:"index"`
	Name  string                            `json:"name"`
	Data  *model.ChannelPreparationResponse `json:"data,omitempty"`
	Ok    bool                              `json:"ok"`
	Error string                            `json:"error,omitempty"`
}

type channelPreparationPromoteResult struct {
	Id        int    `json:"id"`
	ChannelId int    `json:"channel_id,omitempty"`
	Ok        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

func parseOptionalIntQuery(c *gin.Context, name string) (*int, error) {
	value := strings.TrimSpace(c.Query(name))
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseOptionalInt64Query(c *gin.Context, name string) (*int64, error) {
	value := strings.TrimSpace(c.Query(name))
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func defaultChannelPreparationModels(channelType int) string {
	if channelType == 0 {
		channelType = constant.ChannelTypeAnthropic
	}
	models := channelId2Models[channelType]
	if len(models) == 0 {
		models = channelId2Models[constant.ChannelTypeAnthropic]
	}
	return strings.Join(models, ",")
}

func applyChannelPreparationDefaults(preparation *model.ChannelPreparation) {
	if preparation == nil {
		return
	}
	if preparation.Type == 0 {
		preparation.Type = constant.ChannelTypeAnthropic
	}
	if strings.TrimSpace(preparation.Models) == "" {
		preparation.Models = defaultChannelPreparationModels(preparation.Type)
	}
}

func validateChannelPreparationInput(preparation *model.ChannelPreparation, isCreate bool) error {
	if preparation == nil {
		return fmt.Errorf("preparation cannot be empty")
	}
	preparation.Name = strings.TrimSpace(preparation.Name)
	preparation.Key = strings.TrimSpace(preparation.Key)
	if preparation.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if isCreate && preparation.Key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	preparation.Group = model.NormalizeChannelPreparationGroup(preparation.Group)
	applyChannelPreparationDefaults(preparation)
	model.NormalizeDirectAnthropicPreparationModels(preparation)
	if preparation.Remark != nil && len(*preparation.Remark) > 255 {
		return fmt.Errorf("remark is too long")
	}
	if preparation.Setting != nil {
		channel := preparation.ToChannel()
		if err := channel.ValidateSettings(); err != nil {
			return fmt.Errorf("渠道额外设置[channel setting] 格式错误：%s", err.Error())
		}
	}
	return nil
}

func normalizeChannelPreparationCreateGroups(group string, groups []string) []string {
	seen := make(map[string]bool, len(groups)+1)
	normalizedGroups := make([]string, 0, len(groups)+1)
	appendGroup := func(value string) {
		normalized := model.NormalizeChannelPreparationGroup(value)
		if seen[normalized] {
			return
		}
		seen[normalized] = true
		normalizedGroups = append(normalizedGroups, normalized)
	}

	if len(groups) > 0 {
		for _, item := range groups {
			if strings.TrimSpace(item) == "" {
				continue
			}
			appendGroup(item)
		}
	} else {
		appendGroup(group)
	}
	if len(normalizedGroups) == 0 {
		appendGroup(group)
	}
	return normalizedGroups
}

func channelPreparationKeyConflictError(conflict model.ChannelPreparation) error {
	statusText := "待晋升"
	if conflict.Status == model.ChannelPreparationStatusPromoting {
		statusText = "晋升中"
	}
	name := strings.TrimSpace(conflict.Name)
	if name == "" {
		name = "未命名"
	}
	return fmt.Errorf("Key 已存在于备货池%s候选渠道：%s（ID %d，%s，分组 %s）", statusText, conflict.KeyPreview(), conflict.Id, name, model.NormalizeChannelPreparationGroup(conflict.Group))
}

func checkChannelPreparationKeyGroupConflict(key string, group string, excludeID int) error {
	conflicts, err := model.FindActiveChannelPreparationKeyGroupConflicts([]model.ChannelPreparationKeyGroup{{Key: key, Group: group}}, excludeID)
	if err != nil {
		return err
	}
	conflictKey := model.ChannelPreparationKeyGroupConflictKey(key, group)
	if conflict, ok := conflicts[conflictKey]; ok {
		return channelPreparationKeyConflictError(conflict)
	}
	return nil
}

func checkChannelPreparationKeyGroupConflicts(key string, groups []string, excludeID int) error {
	pairs := make([]model.ChannelPreparationKeyGroup, 0, len(groups))
	for _, group := range groups {
		pairs = append(pairs, model.ChannelPreparationKeyGroup{Key: key, Group: group})
	}
	conflicts, err := model.FindActiveChannelPreparationKeyGroupConflicts(pairs, excludeID)
	if err != nil {
		return err
	}
	for _, group := range groups {
		conflictKey := model.ChannelPreparationKeyGroupConflictKey(key, group)
		if conflict, ok := conflicts[conflictKey]; ok {
			return channelPreparationKeyConflictError(conflict)
		}
	}
	return nil
}

func GetChannelPreparations(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("p"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	channelType, err := parseOptionalIntQuery(c, "type")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	status, err := parseOptionalIntQuery(c, "status")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	startTimestamp, err := parseOptionalInt64Query(c, "start_timestamp")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	endTimestamp, err := parseOptionalInt64Query(c, "end_timestamp")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if status == nil {
		pendingStatus := model.ChannelPreparationStatusPending
		status = &pendingStatus
	}
	opts := model.ChannelPreparationListOptions{
		Page:           page,
		PageSize:       pageSize,
		Keyword:        c.Query("keyword"),
		Group:          c.Query("group"),
		Type:           channelType,
		Status:         status,
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		IDSort:         c.Query("id_sort") == "true" || c.Query("id_sort") == "1",
	}
	preparations, total, stats, statusCounts, typeCounts, err := model.GetChannelPreparations(opts)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":         model.ChannelPreparationResponses(preparations),
		"total":         total,
		"page":          opts.Page,
		"page_size":     opts.PageSize,
		"stats":         stats,
		"status_counts": statusCounts,
		"type_counts":   typeCounts,
	})
}

func GetChannelPreparation(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var preparation model.ChannelPreparation
	if err := model.DB.First(&preparation, "id = ?", id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, preparation.ToResponse())
}

func TestChannelPreparation(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var preparation model.ChannelPreparation
	if err := model.DB.First(&preparation, "id = ?", id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	applyChannelPreparationDefaults(&preparation)
	model.NormalizeDirectAnthropicPreparationModels(&preparation)
	channel := preparation.ToChannel()
	testModel := strings.TrimSpace(c.Query("model"))
	endpointType := c.Query("endpoint_type")
	isStream, _ := strconv.ParseBool(c.Query("stream"))
	testUserID, err := resolveChannelTestUserID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tik := time.Now()
	result := testChannel(channel, testUserID, testModel, endpointType, isStream)
	milliseconds := time.Since(tik).Milliseconds()
	consumedTime := float64(milliseconds) / 1000.0
	if result.localErr != nil {
		message := result.localErr.Error()
		go preparation.UpdateTestResult(milliseconds, model.ChannelPreparationTestStatusFailed, message)
		resp := gin.H{
			"success": false,
			"message": message,
			"time":    consumedTime,
		}
		if result.newAPIError != nil {
			resp["error_code"] = result.newAPIError.GetErrorCode()
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	if result.newAPIError != nil {
		message := result.newAPIError.Error()
		go preparation.UpdateTestResult(milliseconds, model.ChannelPreparationTestStatusFailed, message)
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    message,
			"time":       consumedTime,
			"error_code": result.newAPIError.GetErrorCode(),
		})
		return
	}
	go preparation.UpdateResponseTime(milliseconds)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"time":    consumedTime,
	})
}

func AddChannelPreparation(c *gin.Context) {
	var request channelPreparationCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	preparation := request.ChannelPreparation
	groups := normalizeChannelPreparationCreateGroups(preparation.Group, request.Groups)
	preparation.Group = groups[0]
	if err := validateChannelPreparationInput(&preparation, true); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := checkChannelPreparationKeyGroupConflicts(preparation.Key, groups, 0); err != nil {
		common.ApiError(c, err)
		return
	}

	tx := model.DB.Begin()
	if tx.Error != nil {
		common.ApiError(c, tx.Error)
		return
	}
	responses := make([]model.ChannelPreparationResponse, 0, len(groups))
	for _, group := range groups {
		item := preparation
		item.Group = group
		item.NormalizeForCreate()
		if err := tx.Create(&item).Error; err != nil {
			tx.Rollback()
			common.ApiError(c, err)
			return
		}
		response := item.ToResponse()
		responses = append(responses, response)
	}
	if err := tx.Commit().Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if len(responses) == 1 {
		common.ApiSuccess(c, responses[0])
		return
	}
	common.ApiSuccess(c, gin.H{"items": responses, "count": len(responses)})
}

func UpdateChannelPreparation(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var input model.ChannelPreparation
	if err := c.ShouldBindJSON(&input); err != nil {
		common.ApiError(c, err)
		return
	}
	var existing model.ChannelPreparation
	if err := model.DB.First(&existing, "id = ?", id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if existing.Status != model.ChannelPreparationStatusPending {
		common.ApiErrorMsg(c, "只有待晋升的候选渠道可以编辑")
		return
	}
	input.NormalizeForUpdate(&existing)
	if err := validateChannelPreparationInput(&input, false); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := checkChannelPreparationKeyGroupConflict(input.Key, input.Group, existing.Id); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Save(&input).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, input.ToResponse())
}

func DeleteChannelPreparation(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := model.DB.Delete(&model.ChannelPreparation{}, "id = ?", id)
	if result.Error != nil {
		common.ApiError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		common.ApiErrorMsg(c, "候选渠道不存在")
		return
	}
	common.ApiSuccess(c, gin.H{"id": id})
}

func ImportChannelPreparations(c *gin.Context) {
	var request channelPreparationImportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}

	resultsByIndex := make([]channelPreparationImportResult, len(request.Items))
	resultSet := make([]bool, len(request.Items))
	normalizedItems := make([]model.ChannelPreparation, len(request.Items))
	validIndexes := make([]int, 0, len(request.Items))
	validPairs := make([]model.ChannelPreparationKeyGroup, 0, len(request.Items))

	for index, item := range request.Items {
		if strings.TrimSpace(item.Source) == "" {
			item.Source = "batch_import"
		}
		if err := validateChannelPreparationInput(&item, true); err != nil {
			resultsByIndex[index] = channelPreparationImportResult{Index: index, Name: item.Name, Ok: false, Error: err.Error()}
			resultSet[index] = true
			continue
		}
		item.NormalizeForCreate()
		normalizedItems[index] = item
		validIndexes = append(validIndexes, index)
		validPairs = append(validPairs, model.ChannelPreparationKeyGroup{Key: item.Key, Group: item.Group})
	}

	dbConflicts, err := model.FindActiveChannelPreparationKeyGroupConflicts(validPairs, 0)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	seenImportKeys := make(map[string]int, len(validIndexes))
	for _, index := range validIndexes {
		item := normalizedItems[index]
		key := strings.TrimSpace(item.Key)
		conflictKey := model.ChannelPreparationKeyGroupConflictKey(key, item.Group)
		if firstIndex, ok := seenImportKeys[conflictKey]; ok {
			resultsByIndex[index] = channelPreparationImportResult{
				Index: index,
				Name:  item.Name,
				Ok:    false,
				Error: fmt.Sprintf("本次导入重复：第 %d 条已包含相同 Key 和分组", firstIndex+1),
			}
			resultSet[index] = true
			continue
		}
		seenImportKeys[conflictKey] = index

		if conflict, ok := dbConflicts[conflictKey]; ok {
			resultsByIndex[index] = channelPreparationImportResult{Index: index, Name: item.Name, Ok: false, Error: channelPreparationKeyConflictError(conflict).Error()}
			resultSet[index] = true
			continue
		}
		if err := model.DB.Create(&item).Error; err != nil {
			resultsByIndex[index] = channelPreparationImportResult{Index: index, Name: item.Name, Ok: false, Error: err.Error()}
			resultSet[index] = true
			continue
		}
		response := item.ToResponse()
		resultsByIndex[index] = channelPreparationImportResult{Index: index, Name: item.Name, Data: &response, Ok: true}
		resultSet[index] = true
	}

	results := make([]channelPreparationImportResult, 0, len(request.Items))
	for index := range request.Items {
		if resultSet[index] {
			results = append(results, resultsByIndex[index])
		}
	}
	common.ApiSuccess(c, gin.H{"results": results})
}

func promoteChannelPreparation(id int) (int, error) {
	tx := model.DB.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	now := common.GetTimestamp()
	lockResult := tx.Model(&model.ChannelPreparation{}).
		Where("id = ? AND status = ?", id, model.ChannelPreparationStatusPending).
		Updates(map[string]any{
			"status":       model.ChannelPreparationStatusPromoting,
			"updated_time": now,
		})
	if lockResult.Error != nil {
		tx.Rollback()
		return 0, lockResult.Error
	}
	if lockResult.RowsAffected == 0 {
		tx.Rollback()
		return 0, fmt.Errorf("候选渠道不存在或不可晋升")
	}

	var preparation model.ChannelPreparation
	if err := tx.First(&preparation, "id = ?", id).Error; err != nil {
		tx.Rollback()
		return 0, err
	}
	applyChannelPreparationDefaults(&preparation)
	channel := preparation.ToChannel()
	channels, err := createChannelsFromAddRequest(&AddChannelRequest{Mode: "single", Channel: channel}, tx)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	if len(channels) == 0 {
		tx.Rollback()
		return 0, fmt.Errorf("channel cannot be empty")
	}
	channelID := channels[0].Id
	deleteResult := tx.Where("id = ? AND status = ?", id, model.ChannelPreparationStatusPromoting).
		Delete(&model.ChannelPreparation{})
	if deleteResult.Error != nil {
		tx.Rollback()
		return 0, deleteResult.Error
	}
	if deleteResult.RowsAffected == 0 {
		tx.Rollback()
		return 0, fmt.Errorf("候选渠道晋升后删除失败")
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return channelID, nil
}

func PromoteChannelPreparation(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channelID, err := promoteChannelPreparation(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()
	common.ApiSuccess(c, gin.H{"id": id, "channel_id": channelID})
}

func PromoteChannelPreparationsBatch(c *gin.Context) {
	var request channelPreparationBatchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	results := make([]channelPreparationPromoteResult, 0, len(request.Ids))
	succeeded := false
	for _, id := range request.Ids {
		channelID, err := promoteChannelPreparation(id)
		if err != nil {
			results = append(results, channelPreparationPromoteResult{Id: id, Ok: false, Error: err.Error()})
			continue
		}
		succeeded = true
		results = append(results, channelPreparationPromoteResult{Id: id, ChannelId: channelID, Ok: true})
	}
	if succeeded {
		model.InitChannelCache()
		service.ResetProxyClientCache()
	}
	common.ApiSuccess(c, gin.H{"results": results})
}
