package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"kamailio-doorbell/common"
	"net/http"
)

type CallHandler struct {
	channelMap *common.ChannelMap
}

func NewCallHandler(channelMap *common.ChannelMap) *CallHandler {
	return &CallHandler{
		channelMap: channelMap,
	}
}

func (r *CallHandler) makeCall(c *gin.Context) {
	var callRequest map[string]interface{}
	if err := c.BindJSON(&callRequest); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	r.channelMap.AddChannel(callRequest["correlationToken"].(string))

	// forward the json via http
	// Create a new POST request
	jsonData, err := json.Marshal(callRequest)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
	}
	req, err := http.NewRequest("POST", "http://localhost:8080/CALL", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	// Set the appropriate headers
	req.Header.Set("Content-Type", "application/json")

	// Send the request using the default HTTP client
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return
	}
	defer resp.Body.Close()

	// Print the response status and body
	fmt.Println("Response status:", resp.Status)

	response := r.channelMap.WaitForMessage(callRequest["correlationToken"].(string), 10)
	if response == nil {
		c.JSON(500, gin.H{"error": "timeout"})
		return
	}

	c.JSON(200, response)
	r.channelMap.RemoveChannel(callRequest["correlationToken"].(string))
}

func (r *CallHandler) SetHandlers(rg *gin.RouterGroup) {

	rg.POST("/CALL", r.makeCall)
}
