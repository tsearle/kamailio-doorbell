package main

import (
	"github.com/caarlos0/env/v10"
	"github.com/emiago/sipgo"
	"github.com/gin-gonic/gin"
	"kamailio-doorbell/conf"
	"kamailio-doorbell/server"
	"kamailio-doorbell/voip"
	"log"
	"net"
)

func main() {
	var envConfig conf.Config
	if err := env.Parse(&envConfig); err != nil {
		log.Fatal(err)
	}
	// create a gin web server
	r := gin.Default()

	var unicastAddress *string = nil
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		panic(err)
	}
	for _, address := range addrs {
		if unicastAddress != nil && *unicastAddress != "" {
			break
		}

		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				addr := ipnet.IP.String()
				unicastAddress = &addr
			}
		}
	}

	if unicastAddress != nil {
		log.Printf("Using unicast address: %s", *unicastAddress)
	} else {
		panic("No unicast address found")
	}

	// create a SIP Server
	ua, err := sipgo.NewUA(
		sipgo.WithUserAgent("doorbell"), sipgo.WithUserAgentHostname("sipstacks.com"))
	if err != nil {
		panic(err)
	}
	sipServer := voip.NewSipServer(ua, *unicastAddress)
	rtcServer := voip.NewRTCServer()

	group := r.Group("/")
	makeCallHandler := server.NewCallHandler(envConfig, sipServer, rtcServer)
	makeCallHandler.SetHandlers(group)

	// run the web server
	r.Run(":8090")

}
