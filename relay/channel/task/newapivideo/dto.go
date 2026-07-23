package newapivideo

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const requestStateContextKey = "newapi_video_request_state"

type requestState struct {
	OpenAIFields map[string]json.RawMessage
	Duration     *decimal.Decimal
	Seconds      *decimal.Decimal
}

func getRequestState(c *gin.Context) (requestState, error) {
	value, exists := c.Get(requestStateContextKey)
	if !exists {
		return requestState{}, fmt.Errorf("new-api video request state is missing")
	}
	state, ok := value.(requestState)
	if !ok {
		return requestState{}, fmt.Errorf("invalid new-api video request state")
	}
	return state, nil
}
