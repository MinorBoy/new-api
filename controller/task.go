package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		RequestModel:   c.Query("request_model"),
		UserID:         c.Query("user_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
	}

	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllTasks(queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, true))
	common.ApiSuccess(c, pageInfo)
}

func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		RequestModel:   c.Query("request_model"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}

	items := model.TaskGetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllUserTask(userId, queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, false))
	common.ApiSuccess(c, pageInfo)
}

func GetAllTaskFilterOptions(c *gin.Context) {
	getTaskFilterOptions(c, 0, true)
}

func GetUserTaskFilterOptions(c *gin.Context) {
	getTaskFilterOptions(c, c.GetInt("id"), false)
}

func getTaskFilterOptions(c *gin.Context, userID int, includeAdminDimensions bool) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	options, err := model.GetTaskFilterOptions(userID, startTimestamp, endTimestamp, includeAdminDimensions)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	response := dto.TaskFilterOptions{
		Statuses:      make([]string, len(options.Statuses)),
		RequestModels: options.RequestModels,
	}
	for i, status := range options.Statuses {
		response.Statuses[i] = string(status)
	}

	if includeAdminDimensions {
		response.Channels = make([]dto.TaskFilterChannelOption, 0, len(options.ChannelIDs))
		if len(options.ChannelIDs) > 0 {
			channels, err := model.GetChannelsByIds(options.ChannelIDs)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			channelNames := make(map[int]string, len(channels))
			for _, channel := range channels {
				channelNames[channel.Id] = channel.Name
			}
			for _, channelID := range options.ChannelIDs {
				response.Channels = append(response.Channels, dto.TaskFilterChannelOption{
					ID:   channelID,
					Name: channelNames[channelID],
				})
			}
		}
		response.Users = make([]dto.TaskFilterUserOption, 0, len(options.UserIDs))
		for _, optionUserID := range options.UserIDs {
			userOption := dto.TaskFilterUserOption{ID: optionUserID}
			user, err := model.GetUserCache(optionUserID)
			if err == nil {
				userOption.Username = user.Username
			}
			response.Users = append(response.Users, userOption)
		}
	}

	common.ApiSuccess(c, response)
}

func tasksToDto(tasks []*model.Task, fillUser bool) []*dto.TaskDto {
	var userIdMap map[int]*model.UserBase
	if fillUser {
		userIdMap = make(map[int]*model.UserBase)
		userIds := types.NewSet[int]()
		for _, task := range tasks {
			userIds.Add(task.UserId)
		}
		for _, userId := range userIds.Items() {
			cacheUser, err := model.GetUserCache(userId)
			if err == nil {
				userIdMap[userId] = cacheUser
			}
		}
	}
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if fillUser {
			if user, ok := userIdMap[task.UserId]; ok {
				task.Username = user.Username
			}
		}
		result[i] = relay.TaskModel2Dto(task, fillUser)
	}
	return result
}
