package server

import (
	"github.com/gin-gonic/gin"
	"github.com/pion/sdp/v3"
	"kamailio-doorbell/conf"
	"kamailio-doorbell/voip"
	"log"
)

type CallHandler struct {
	sipServer *voip.SipServer
	rtcServer *voip.RTCServer
	conf      conf.Config
}

func NewCallHandler(env conf.Config, sipServer *voip.SipServer, rtcServer *voip.RTCServer) *CallHandler {
	return &CallHandler{
		sipServer: sipServer,
		rtcServer: rtcServer,
		conf:      env,
	}
}

func (r *CallHandler) makeCall(c *gin.Context) {
	var callRequest map[string]interface{}
	if err := c.BindJSON(&callRequest); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	sdpOffer := callRequest["offer"].(string)
	correlationToken := callRequest["endpointId"].(string) //callRequest["correlationToken"].(string)
	endpointId := callRequest["endpointId"].(string)
	apiKey := callRequest["apiKey"].(string)

	if apiKey != r.conf.ApiKey {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	rtcCall, response, err := r.rtcServer.NewCall(correlationToken, sdpOffer)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	log.Printf("RTC Call created: response body \n%s\n", response)
	parsedSDP := sdp.SessionDescription{}
	err = parsedSDP.Unmarshal([]byte(sdpOffer))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	sipCall, err := r.sipServer.SendInvite(endpointId, correlationToken, parsedSDP)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	sipCall.Audio.SetWriteHandler(func(data []byte) { _, _ = rtcCall.WriteAudio(data) })
	sipCall.Video.SetWriteHandler(func(data []byte) { _, _ = rtcCall.WriteVideo(data) })
	rtcCall.SetAudioWriteHandler(func(data []byte) { _, _ = sipCall.Audio.Write(data) })
	rtcCall.SetVideoWriteHandler(func(data []byte) { _, _ = sipCall.Video.Write(data) })

	/*
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
		}*/

	c.JSON(200, gin.H{"sdp": response})
}

func (r *CallHandler) hangupCall(c *gin.Context) {
	var callRequest map[string]interface{}
	if err := c.BindJSON(&callRequest); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	apiKey := callRequest["apiKey"].(string)

	if apiKey != r.conf.ApiKey {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	correlationToken := callRequest["endpointId"].(string) //callRequest["correlationToken"].(string)
	if err := r.rtcServer.HangupCall(correlationToken); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if err := r.sipServer.SendBye(correlationToken); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (r *CallHandler) SetHandlers(rg *gin.RouterGroup) {

	rg.POST("/CALL", r.makeCall)
	rg.POST("/BYE", r.hangupCall)
}
