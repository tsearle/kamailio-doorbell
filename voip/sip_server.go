package server

import (
	"github.com/emiago/sipgo"
	"kamailio-doorbell/common"
)

type SipServer struct {
	channelMap *common.ChannelMap
	ua         *sipgo.UserAgent
	uas        *sipgo.Server
}

func NewSipServer(channelMap *common.ChannelMap, ua *sipgo.UserAgent) *SipServer {
	ss := &SipServer{
		channelMap: channelMap,
		ua:         ua,
	}
	var err error
	ss.uas, err = sipgo.NewServer(ua)
	if err != nil {
		panic(err)
	}
	return ss
}

func (ss *SipServer) HandleRegister(req *sipgo.Request) {

}
