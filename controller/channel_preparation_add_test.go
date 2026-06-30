package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type addChannelPreparationTestResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func postAddChannelPreparation(t *testing.T, payload any) addChannelPreparationTestResponse {
	t.Helper()
	body, err := common.Marshal(payload)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/preparations", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	AddChannelPreparation(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var response addChannelPreparationTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestAddChannelPreparationCreatesOneRecordPerRequestedGroup(t *testing.T) {
	setupModelListControllerTestDB(t)

	response := postAddChannelPreparation(t, map[string]any{
		"type":    1,
		"name":    "multi group candidate",
		"key":     "sk-multi-group",
		"groups":  []string{"default", "vip", "vip", ""},
		"balance": 3.5,
	})
	require.True(t, response.Success, response.Message)

	var preparations []model.ChannelPreparation
	require.NoError(t, model.DB.Order("id asc").Find(&preparations, "key = ?", "sk-multi-group").Error)
	require.Len(t, preparations, 2)
	require.Equal(t, "default", preparations[0].Group)
	require.Equal(t, "vip", preparations[1].Group)
	require.Equal(t, model.ChannelPreparationStatusPending, preparations[0].Status)
	require.Equal(t, model.ChannelPreparationStatusPending, preparations[1].Status)
}

func TestAddChannelPreparationRejectsDuplicateKeyInSameGroupOnly(t *testing.T) {
	setupModelListControllerTestDB(t)
	require.NoError(t, model.DB.Create(&model.ChannelPreparation{
		Type:   1,
		Name:   "existing",
		Key:    "sk-same-key",
		Group:  "default",
		Status: model.ChannelPreparationStatusPending,
	}).Error)

	allowed := postAddChannelPreparation(t, map[string]any{
		"type":  1,
		"name":  "same key vip",
		"key":   "sk-same-key",
		"group": "vip",
	})
	require.True(t, allowed.Success, allowed.Message)

	rejected := postAddChannelPreparation(t, map[string]any{
		"type":  1,
		"name":  "same key default",
		"key":   "sk-same-key",
		"group": "default",
	})
	require.False(t, rejected.Success)
	require.Contains(t, rejected.Message, "分组 default")
}
