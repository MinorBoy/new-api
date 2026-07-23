package relay

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const seedanceTaskScanBatchSize = 200

type seedanceTaskListResponse struct {
	Items []map[string]interface{} `json:"items"`
	Total int                      `json:"total"`
}

// SeedanceTaskFetch serves the ARK task lookup endpoints. The public task ID
// is the only identifier accepted from clients; upstream IDs remain private
// in TaskPrivateData and are never used for lookup.
func SeedanceTaskFetch(c *gin.Context) ([]byte, *dto.TaskError) {
	if strings.TrimSpace(c.Param("task_id")) != "" {
		task, exists, err := seedanceFetchTaskByID(c)
		if err != nil {
			return nil, service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		}
		if !exists {
			return nil, service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusNotFound)
		}
		response, err := seedanceTaskResponse(task)
		if err != nil {
			return nil, service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
		}
		body, err := common.Marshal(response)
		if err != nil {
			return nil, service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
		}
		return body, nil
	}

	response, err := seedanceTaskList(c)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
	}
	body, err := common.Marshal(response)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return body, nil
}

func seedanceFetchTaskByID(c *gin.Context) (*model.Task, bool, error) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		return nil, false, nil
	}
	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil || !exists {
		return task, exists, err
	}
	if !isSeedanceTaskPlatform(task.Platform) {
		return nil, false, nil
	}
	return task, true, nil
}

func seedanceTaskList(c *gin.Context) (*seedanceTaskListResponse, error) {
	pageNum := boundedSeedancePage(c.Query("page_num"), 1, 500, 1)
	pageSize := boundedSeedancePage(c.Query("page_size"), 20, 100, 20)
	query := model.DB.Model(&model.Task{}).
		Where("user_id = ?", c.GetInt("id")).
		Where("platform IN ?", seedanceTaskPlatformValues()).
		Where("submit_time >= ?", time.Now().Add(-7*24*time.Hour).Unix())

	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", seedanceInternalStatus(status))
	}
	if taskIDs := seedanceTaskIDs(c); len(taskIDs) > 0 {
		query = query.Where("task_id IN ?", taskIDs)
	}

	modelFilter := strings.TrimSpace(c.Query("filter.model"))
	serviceTierFilter := strings.TrimSpace(c.Query("filter.service_tier"))
	if modelFilter == "" {
		modelFilter = strings.TrimSpace(c.Query("filter[model]"))
	}
	if serviceTierFilter == "" {
		serviceTierFilter = strings.TrimSpace(c.Query("filter[service_tier]"))
	}

	if modelFilter == "" && serviceTierFilter == "" {
		var total int64
		if err := query.Count(&total).Error; err != nil {
			return nil, err
		}
		var tasks []*model.Task
		err := query.Order("id DESC").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&tasks).Error
		if err != nil {
			return nil, err
		}
		items := make([]map[string]interface{}, 0, len(tasks))
		for _, task := range tasks {
			item, err := seedanceTaskResponse(task)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return &seedanceTaskListResponse{Items: items, Total: int(total)}, nil
	}

	return seedanceFilteredTaskList(query, pageNum, pageSize, modelFilter, serviceTierFilter)
}

func seedanceFilteredTaskList(query *gorm.DB, pageNum, pageSize int, modelFilter, serviceTierFilter string) (*seedanceTaskListResponse, error) {
	wantStart := (pageNum - 1) * pageSize
	wantEnd := wantStart + pageSize
	matched := 0
	items := make([]map[string]interface{}, 0, pageSize)
	var lastID int64
	hasLastID := false

	for {
		batchQuery := query.Order("id DESC").Limit(seedanceTaskScanBatchSize)
		if hasLastID {
			batchQuery = batchQuery.Where("id < ?", lastID)
		}
		var batch []*model.Task
		if err := batchQuery.Find(&batch).Error; err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, task := range batch {
			lastID = task.ID
			hasLastID = true
			if !seedanceTaskMatchesJSONFilter(task, modelFilter, serviceTierFilter) {
				continue
			}
			if matched >= wantStart && matched < wantEnd {
				item, err := seedanceTaskResponse(task)
				if err != nil {
					return nil, err
				}
				items = append(items, item)
			}
			matched++
		}
		if len(batch) < seedanceTaskScanBatchSize {
			break
		}
	}
	return &seedanceTaskListResponse{Items: items, Total: matched}, nil
}

func seedanceTaskMatchesJSONFilter(task *model.Task, modelFilter, serviceTierFilter string) bool {
	var data map[string]interface{}
	if len(task.Data) > 0 {
		_ = common.Unmarshal(task.Data, &data)
	}
	if modelFilter != "" {
		modelName := task.Properties.OriginModelName
		if modelName == "" {
			modelName = task.Properties.UpstreamModelName
		}
		if modelName == "" {
			if value, ok := data["model"].(string); ok {
				modelName = value
			}
		}
		if modelName != modelFilter {
			return false
		}
	}
	if serviceTierFilter != "" {
		serviceTier := ""
		serviceTier, _ = data["service_tier"].(string)
		if serviceTier == "" {
			if wrapperData, ok := data["data"].(map[string]interface{}); ok {
				if nestedData, ok := wrapperData["data"].(map[string]interface{}); ok {
					serviceTier, _ = nestedData["service_tier"].(string)
				}
			}
		}
		if serviceTier == "" {
			if billingContext := task.PrivateData.BillingContext; billingContext != nil {
				serviceTier = billingContext.ServiceTier
			}
		}
		if serviceTier == "" {
			serviceTier = "default"
		}
		if serviceTier != serviceTierFilter {
			return false
		}
	}
	return true
}

func seedanceTaskResponse(task *model.Task) (map[string]interface{}, error) {
	response := make(map[string]interface{})
	adaptor := GetTaskAdaptor(task.Platform)
	if converter, ok := adaptor.(channel.ArkVideoTaskConverter); ok {
		converted, err := converter.ConvertToArkVideoTask(task)
		if err != nil {
			return nil, err
		}
		if err := common.Unmarshal(converted, &response); err != nil {
			return nil, fmt.Errorf("unmarshal converted task data: %w", err)
		}
	} else if task.Platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)) {
		return nil, fmt.Errorf("new-api video task adaptor must implement ARK conversion")
	} else if len(task.Data) > 0 {
		if err := common.Unmarshal(task.Data, &response); err != nil {
			return nil, fmt.Errorf("unmarshal task data: %w", err)
		}
	}
	response["id"] = task.TaskID
	publicStatus := seedanceTaskStatus(task.Status)
	if upstreamStatus, ok := response["status"].(string); ok && task.Status == model.TaskStatusFailure {
		switch strings.ToLower(upstreamStatus) {
		case "expired", "cancelled":
			publicStatus = strings.ToLower(upstreamStatus)
		}
	}
	response["status"] = publicStatus
	if modelValue, exists := response["model"]; !exists || modelValue == "" {
		modelName := task.Properties.OriginModelName
		if modelName == "" {
			modelName = task.Properties.UpstreamModelName
		}
		response["model"] = modelName
	}
	if _, exists := response["created_at"]; !exists {
		createdAt := task.SubmitTime
		if createdAt == 0 {
			createdAt = task.CreatedAt
		}
		response["created_at"] = createdAt
	}
	if _, exists := response["updated_at"]; !exists {
		response["updated_at"] = task.UpdatedAt
	}
	if task.Status == model.TaskStatusSuccess {
		content, ok := response["content"].(map[string]interface{})
		if !ok {
			content = make(map[string]interface{})
		}
		videoURL, _ := content["video_url"].(string)
		if videoURL == "" && task.PrivateData.ResultURL != "" {
			content["video_url"] = task.PrivateData.ResultURL
			response["content"] = content
		}
	}
	return response, nil
}

func isSeedanceTaskPlatform(platform constant.TaskPlatform) bool {
	switch platform {
	case constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVolcEngine)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDimensio)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)):
		return true
	default:
		return false
	}
}

func seedanceTaskPlatformValues() []string {
	candidates := []constant.TaskPlatform{
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVolcEngine)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDimensio)),
		constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
	}
	values := make([]string, 0, len(candidates))
	for _, platform := range candidates {
		if isSeedanceTaskPlatform(platform) {
			values = append(values, string(platform))
		}
	}
	return values
}

func seedanceTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusInProgress:
		return "running"
	case model.TaskStatusQueued, model.TaskStatusSubmitted, model.TaskStatusNotStart:
		return "queued"
	default:
		return "queued"
	}
}

func seedanceInternalStatus(status string) model.TaskStatus {
	switch strings.ToLower(status) {
	case "succeeded", "success":
		return model.TaskStatusSuccess
	case "failed", "failure":
		return model.TaskStatusFailure
	case "running", "processing", "in_progress":
		return model.TaskStatusInProgress
	case "queued", "pending", "submitted":
		return model.TaskStatusQueued
	default:
		return model.TaskStatus(status)
	}
}

func seedanceTaskIDs(c *gin.Context) []string {
	values := c.Request.URL.Query()
	var raw []string
	for _, key := range []string{"filter.task_ids", "filter.task_ids[]", "filter[task_ids]", "filter[task_ids][]"} {
		raw = append(raw, values[key]...)
	}
	seen := make(map[string]struct{})
	ids := make([]string, 0, len(raw))
	for _, value := range raw {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, exists := seen[part]; exists {
				continue
			}
			seen[part] = struct{}{}
			ids = append(ids, part)
		}
	}
	return ids
}

func boundedSeedancePage(raw string, fallback, maximum, defaultValue int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 || value > maximum {
		return defaultValue
	}
	return value
}
