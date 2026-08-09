package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func PreviewGroupRoutingProfileSummaries(c *gin.Context) {
	var request struct {
		Profiles map[string]ratio_setting.GroupRoutingRequirements `json:"profiles"`
	}
	if err := common.DecodeJsonStrict(c.Request.Body, &request); err != nil {
		writeGroupRoutingProfileError(c, &service.GroupRoutingProfileError{Code: service.GroupRoutingProfileErrorInvalid, Err: err})
		return
	}
	if request.Profiles == nil {
		writeGroupRoutingProfileError(c, &service.GroupRoutingProfileError{
			Code: service.GroupRoutingProfileErrorInvalid,
			Err:  errors.New("profiles are required"),
		})
		return
	}
	summaries, err := service.PreviewGroupRoutingProfileSummaries(request.Profiles)
	if err != nil {
		writeGroupRoutingProfileError(c, err)
		return
	}
	common.ApiSuccess(c, summaries)
}

func PreviewGroupRoutingProfileTargets(c *gin.Context) {
	var request service.GroupRoutingProfilePreviewInput
	if err := common.DecodeJsonStrict(c.Request.Body, &request); err != nil {
		writeGroupRoutingProfileError(c, &service.GroupRoutingProfileError{Code: service.GroupRoutingProfileErrorInvalid, Err: err})
		return
	}
	page, err := service.PreviewGroupRoutingProfile(request)
	if err != nil {
		writeGroupRoutingProfileError(c, err)
		return
	}
	common.ApiSuccess(c, page)
}

func writeGroupRoutingProfileError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := service.GroupRoutingProfileErrorPreview
	var profileErr *service.GroupRoutingProfileError
	if errors.As(err, &profileErr) {
		code = profileErr.Code
		switch profileErr.Code {
		case service.GroupRoutingProfileErrorInvalid:
			status = http.StatusBadRequest
		case service.GroupRoutingProfileErrorUnavailable:
			status = http.StatusConflict
		case service.GroupRoutingProfileErrorPreview:
			status = http.StatusInternalServerError
		default:
			code = service.GroupRoutingProfileErrorPreview
		}
	}
	c.JSON(status, gin.H{
		"success": false,
		"message": err.Error(),
		"code":    code,
	})
}
