package main

import (
	"github.com/gin-gonic/gin"
	"kamailio-doorbell/common"
	"kamailio-doorbell/server"
)

func main() {

	// create a gin web server
	r := gin.Default()

	channelMap := common.NewChannelMap()
	group := r.Group("/")
	makeCallHandler := server.NewCallHandler(channelMap)
	makeCallHandler.SetHandlers(group)
	answerCallHandler := server.NewAnswerHandler(channelMap)
	answerCallHandler.SetHandlers(group)

	// run the web server
	r.Run(":8090")

}
